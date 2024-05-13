package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	ipfsClusterClientApi "github.com/ipfs-cluster/ipfs-cluster/api"
	ipfsCluster "github.com/ipfs-cluster/ipfs-cluster/api/rest/client"
	"github.com/ipfs/go-cid"
)

type ManifestBatchUploadRequest struct {
	Cid               []string           `json:"cid"`
	PoolID            int                `json:"pool_id"`
	ReplicationFactor []int              `json:"replication_factor"`
	ManifestMetadata  []ManifestMetadata `json:"manifest_metadata"`
}

type ManifestBatchUploadResponse struct {
	PoolID int      `json:"pool_id"`
	Storer string   `json:"storer"`
	Cid    []string `json:"cid"`
}

type ManifestMetadata struct {
	Job ManifestJob `json:"job"`
}

type ManifestJob struct {
	Work   string `json:"work"`
	Engine string `json:"engine"`
	Uri    string `json:"uri"`
}

var (
	blockchainEndpoint = "http://127.0.0.1:4000" // Endpoint for the blockchain service
	ipfsClusterAPI     ipfsCluster.Client
)

func init() {
	var err error
	ipfsClusterConfig := ipfsCluster.Config{}
	ipfsClusterAPI, err = ipfsCluster.NewDefaultClient(&ipfsClusterConfig)
	if err != nil {
		log.Fatalf("Error creating IPFS Cluster client: %v", err)
	}
}

func handleManifestBatchUpload(w http.ResponseWriter, r *http.Request) {
	var req ManifestBatchUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// First, add CIDs to the blockchain
	resp, statusCode, err := callBlockchain("POST", "fula-manifest-batch_upload", req)
	if err != nil {
		log.Println("Failed to register CIDs on blockchain:", err)
		http.Error(w, "Blockchain interaction failed", statusCode)
		return
	}

	// Assuming blockchain response validates the request to proceed with pinning
	var blockchainResp ManifestBatchUploadResponse
	if err := json.Unmarshal(resp, &blockchainResp); err != nil {
		log.Println("Failed to decode blockchain response:", err)
		http.Error(w, "Failed to decode blockchain response", http.StatusInternalServerError)
		return
	}

	// Proceed with IPFS Cluster pinning
	pinOptions := ipfsClusterClientApi.PinOptions{
		Mode: 0,
	}

	for _, cidStr := range blockchainResp.Cid {
		c, _ := cid.Decode(cidStr)
		_, err := ipfsClusterAPI.Pin(context.Background(), ipfsClusterClientApi.NewCid(c), pinOptions)
		if err != nil {
			log.Printf("Failed to pin CID %s: %v", cidStr, err)
			continue
		}
	}

	fmt.Fprintf(w, "CIDs pinned successfully: %v", blockchainResp.Cid)
}

func callBlockchain(method, action string, payload interface{}) ([]byte, int, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	req, err := http.NewRequest(method, blockchainEndpoint+"/"+action, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}

	return respBody, resp.StatusCode, nil
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/api/v0/manifests", handleManifestBatchUpload).Methods("POST")

	log.Println("Server is running on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", r))
}
