package utils

import (
	"encoding/json"
	"net/http"
)

type ErrorResponse struct {
	Error   string         `json:"error"`
	Details map[string]any `json:"details,omitempty"`
}

func WriteResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func WriteError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}

const (
	MaxFileBytes = 1 << 20 // 1MB
	SniffLen     = 512
	NullByte     = 0x00
)
