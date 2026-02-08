package utils

import (
	"github.com/gofiber/fiber/v2"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

func WriteError(c *fiber.Ctx, status int, message string, detail ...any) error {
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
	return c.Status(status).JSON(resp)
}
