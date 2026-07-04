package utils

import (
	"encoding/json/v2"
	"errors"
	"net/http"

	"chi-server/internal/consts"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

func WriteResponse(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.MarshalWrite(w, data); err != nil {
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
	if err := json.MarshalWrite(w, resp); err != nil {
		return
	}
}

// WriteBodyError renders a JSON request-body decode failure. A body over the
// global cap surfaces as *http.MaxBytesError from the MaxBytesReader-wrapped
// body and becomes 413 "request body too large"; everything else is a plain
// 400 "invalid JSON body".
func WriteBodyError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		WriteError(w, http.StatusRequestEntityTooLarge, consts.ErrRequestTooLarge)
		return
	}
	WriteError(w, http.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
}
