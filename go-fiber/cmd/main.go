package main

import "github.com/gofiber/fiber/v2"

func main() {
	app := fiber.New()

	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"message": "Hello, World!"})
	})

	app.Get("/ping", func(c *fiber.Ctx) error {
		return c.SendString("PONG!")
	})

	app.Listen(":3000")
}
