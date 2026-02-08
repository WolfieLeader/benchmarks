package utils

import (
	"github.com/gin-gonic/gin"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
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
