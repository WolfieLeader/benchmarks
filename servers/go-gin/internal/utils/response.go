package utils

import (
	"encoding/json/jsontext"
	"encoding/json/v2"

	"github.com/gin-gonic/gin"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// WriteResponse marshals data with encoding/json/v2 straight to the response
// writer — a handler-level stand-in for c.JSON, whose render engine only swaps
// between bundled encoders via build tags (no stdlib json/v2 among them).
// Headers/status mirror c.JSON: gin defers the actual header write until the
// first body write, so setting status before marshaling matches today's bytes.
func WriteResponse(c *gin.Context, status int, data any) {
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Status(status)
	if err := json.MarshalWrite(c.Writer, data); err != nil {
		return
	}
}

// BindJSON decodes the request body with encoding/json/v2 — a handler-level
// stand-in for c.ShouldBindJSON. AllowDuplicateNames keeps decoding aligned
// with every other server in the suite: duplicate keys take the last value
// (JSON.parse semantics in the JS/Python stacks), where json/v2 alone would
// reject them by default.
func BindJSON(c *gin.Context, out any) error {
	return json.UnmarshalRead(c.Request.Body, out, jsontext.AllowDuplicateNames(true))
}

func WriteError(c *gin.Context, status int, message string, detail ...any) {
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
	WriteResponse(c, status, resp)
}
