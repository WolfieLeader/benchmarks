package utils

import (
	"github.com/gofiber/fiber/v2"
)

type ErrorResponse struct {
	Error   string         `json:"error"`
	Details map[string]any `json:"details,omitempty"`
}

func WriteResponse(c *fiber.Ctx, status int, data any) error {
	c.Status(status)
	return c.JSON(data)
}

func WriteError(c *fiber.Ctx, status int, message string) error {
	c.Status(status)
	return c.JSON(ErrorResponse{Error: message})
}

const (
	MaxFileBytes = 1 << 20 // 1MB
	SniffLen     = 512
	NullByte     = 0x00
)
