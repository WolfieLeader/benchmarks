package utils

import (
	"github.com/gin-gonic/gin"
)

type ErrorResponse struct {
	Error   string         `json:"error"`
	Details map[string]any `json:"details,omitempty"`
}

func WriteResponse(c *gin.Context, status int, data any) {
	c.JSON(status, data)
}

func WriteError(c *gin.Context, status int, message string) {
	c.JSON(status, ErrorResponse{Error: message})
}

// Constants for error messages
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

// Constants for limits
const (
	MaxFileBytes = 1 << 20 // 1MB
	SniffLen     = 512
	NullByte     = 0x00
)
