package utils

import (
	"encoding/json"
	"net/http"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

func WriteResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		return
	}
}

func WriteError(w http.ResponseWriter, status int, message string, detail ...any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := ErrorResponse{Error: message}
	if len(detail) > 0 {
		switch v := detail[0].(type) {
		case string:
			if v != "" {
				resp.Details = v
			}
		case error:
			if v != nil {
				resp.Details = v.Error()
			}
		}
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		return
	}
}
