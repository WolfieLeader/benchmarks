package utils

import (
	"github.com/gofiber/fiber/v2"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

func WriteResponse(c *fiber.Ctx, status int, data any) error {
	c.Status(status)
	return c.JSON(data)
}

func WriteError(c *fiber.Ctx, status int, message string, detail ...any) error {
	c.Status(status)
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
	return c.JSON(resp)
}

const (
	MaxFileBytes = 1 << 20 // 1MB
	SniffLen     = 512
	NullByte     = 0x00
)
