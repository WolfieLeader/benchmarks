package app

import (
	"encoding/json"

	"fiber-server/internal/config"
	"fiber-server/internal/database"
	"fiber-server/internal/routes"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

type App struct {
	env    *config.Env
	router *fiber.App
}

func New() *App {
	r := fiber.New(fiber.Config{
		JSONEncoder: json.Marshal,
		JSONDecoder: json.Unmarshal,
	})

	env := config.LoadEnv()

	database.InitializeConnections(env)

	if env.ENV != "prod" {
		r.Use(logger.New())
	}
	r.Use(recover.New())

	r.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"hello": "world"})
	})
	r.Get("/health", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	routes.RegisterParams(r.Group("/params"))
	routes.RegisterDb(r.Group("/db"), env)

	r.Use(func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	})

	return &App{env: env, router: r}
}
