package utils

import (
	"github.com/gin-gonic/gin"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

func WriteResponse(c *gin.Context, status int, data any) {
	c.JSON(status, data)
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
	c.JSON(status, resp)
}

const (
	ErrInvalidJSON         = "invalid JSON body"
	ErrInvalidForm         = "invalid form data"
	ErrInvalidMultipart    = "invalid multipart form data"
	ErrFileNotFound        = "file not found in form data"
	ErrFileSizeExceeded    = "file size exceeds limit"
	ErrInvalidFileType     = "only text/plain files are allowed"
	ErrNotPlainText        = "file does not look like plain text"
	ErrNotFound            = "not found"
	ErrInternal            = "internal error"
	ErrUnableToRead        = "unable to read file"
	ErrUnableToOpen        = "unable to open uploaded file"
	ErrUnableToReadContent = "unable to read file content"
)

const (
	MaxFileBytes = 1 << 20 // 1MB
	SniffLen     = 512
	NullByte     = 0x00
)
