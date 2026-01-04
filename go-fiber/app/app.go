package app

import "github.com/gofiber/fiber/v2"

type App struct {
	env    *Env
	router *fiber.App
}

func New() *App {
	r := fiber.New()

	r.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "Hello, World!"})
	})

	r.Get("/ping", func(c *fiber.Ctx) error {
		return c.SendString("PONG!")
	})

	return &App{router: r}
}
