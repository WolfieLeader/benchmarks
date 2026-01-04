package app

import (
	"fiber-server/internals/routes"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

type App struct {
	env    *Env
	router *fiber.App
}

func New() *App {
	r := fiber.New()

	r.Use(logger.New())
	r.Use(recover.New())

	r.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "Hello, World!"})
	})
	r.Get("/ping", func(c *fiber.Ctx) error {
		return c.SendString("PONG!")
	})

	routes.RegisterParams(r.Group("/params"))

	return &App{router: r}
}
