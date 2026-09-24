package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"net/http"
	"time"
)

type ProxyTrafficResponse struct {
	UploadBytes   uint64    `json:"upload_bytes"`
	DownloadBytes uint64    `json:"download_bytes"`
	TotalBytes    uint64    `json:"total_bytes"`
	StartedAt     time.Time `json:"started_at"`
}

func GSApiProxyTrafficGET(
	w http.ResponseWriter,
	_ *http.Request,
	snapshot func() (uploadBytes, downloadBytes uint64, startedAt time.Time),
) {
	response := ProxyTrafficResponse{}
	if snapshot != nil {
		response.UploadBytes, response.DownloadBytes, response.StartedAt = snapshot()
		response.TotalBytes = response.UploadBytes + response.DownloadBytes
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(response)
}
