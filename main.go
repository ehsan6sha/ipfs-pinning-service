package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	ipfsClusterClientApi "github.com/ipfs-cluster/ipfs-cluster/api"
	ipfsCluster "github.com/ipfs-cluster/ipfs-cluster/api/rest/client"
	"github.com/ipfs/go-cid"
)

// Define structures for manifest upload requests and responses
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

// Global variables
var (
	blockchainEndpoint = "http://127.0.0.1:4000" // Blockchain service endpoint
	ipfsClusterAPI     ipfsCluster.Client
	authTokens         = map[string]bool{"your-secret-token": true} // Example token storage
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
		Mode: ipfsClusterClientApi.PinModeRecursive,
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

func handleIPFSPinRequest(w http.ResponseWriter, r *http.Request) {
    var pinRequest struct {
        CID     string `json:"cid"`
        Name    string `json:"name"`
        Origins []string `json:"origins"`
        Meta    map[string]string `json:"meta"`
    }
    
    if err := json.NewDecoder(r.Body).Decode(&pinRequest); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Translate to internal format
    internalRequest := ManifestBatchUploadRequest{
        Cid: []string{pinRequest.CID},
        PoolID: 0, // Default or derived value
        ReplicationFactor: []int{1}, // Default value
        ManifestMetadata: []ManifestMetadata{
            {
                Job: ManifestJob{
                    Work:   "storage", // Default work type
                    Engine: "IPFS",
                    Uri:    pinRequest.CID,
                },
            },
        },
    }

    // Now pass to existing blockchain call
    resp, statusCode, err := callBlockchain("POST", "fula-manifest-batch_upload", internalRequest)
    if err != nil || statusCode != http.StatusOK {
        http.Error(w, "Failed to process pin request: "+err.Error(), statusCode)
        return
    }

    // Assume blockchain response is in a suitable format
    var blockchainResp ManifestBatchUploadResponse
    if err := json.Unmarshal(resp, &blockchainResp); err != nil {
        http.Error(w, "Failed to parse blockchain response", http.StatusInternalServerError)
        return
    }

    // Translate blockchain response to IPFS Pinning Service format
    ipfsPinStatus := translateToIPFSPinStatus(blockchainResp, pinRequest)
    
    json.NewEncoder(w).Encode(ipfsPinStatus)
}

func translateToIPFSPi


func main() {
    r := mux.NewRouter()
    apiRouter := r.PathPrefix("/api/v0").Subrouter()
    apiRouter.Use(authenticateMiddleware)
    apiRouter.HandleFunc("/pins", handleIPFSPinRequest).Methods("POST")

    log.Println("Server is running on port 8008...")
    log.Fatal(http.ListenAndServe(":8008", apiRouter))
}

func authenticateMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" || !strings.HasPrefix(token, "Bearer ") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		actualToken := strings.TrimPrefix(token, "Bearer ")
		if _, exists := authTokens[actualToken]; !exists {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
