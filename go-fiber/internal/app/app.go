package app

import (
	"encoding/json/v2"
	"fiber-server/internal/routes"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

type App struct {
	env    *Env
	router *fiber.App
}

func New() *App {
	r := fiber.New(fiber.Config{
		JSONEncoder: func(v any) ([]byte, error) { return json.Marshal(v) },
		JSONDecoder: func(data []byte, v any) error { return json.Unmarshal(data, v) },
	})

	env := LoadEnv()

	if env.ENV != "prod" {
		r.Use(logger.New())
	}
	r.Use(recover.New())

	r.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "Hello, World!"})
	})
	r.Get("/health", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	routes.RegisterParams(r.Group("/params"))

	r.Use(func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	})

	return &App{env: env, router: r}
}
