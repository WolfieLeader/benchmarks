package app

import (
	"encoding/json/jsontext"
	"encoding/json/v2"

	"fiber-server/internal/config"
	"fiber-server/internal/consts"
	"fiber-server/internal/database"
	"fiber-server/internal/routes"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"
)

type App struct {
	env    *config.Env
	router *fiber.App
}

func New() *App {
	// json/v2's Marshal/Unmarshal take variadic options, so they don't satisfy
	// fiber's non-variadic encoder/decoder func types directly — wrap them.
	// AllowDuplicateNames keeps decoding aligned with every other server in the
	// suite: duplicate keys take the last value (JSON.parse semantics in the
	// JS/Python stacks), where json/v2 alone would reject them by default.
	// No StructValidator here, deliberately: with it nil, c.Bind().Body() only
	// decodes (manual mode — bind errors return to the handler, no auto-400),
	// and validation stays in the handlers via go-playground like every other
	// server. Setting one would make Bind() auto-validate and change the
	// status/shape of error responses on the body routes.
	r := fiber.New(fiber.Config{
		// Global request-body cap so no route can read an unbounded body. The file
		// route enforces its own smaller 1MB limit; a body under this global cap
		// still reaches that check and returns its own 413.
		BodyLimit:   consts.MaxRequestBytes,
		JSONEncoder: func(v any) ([]byte, error) { return json.Marshal(v) },
		JSONDecoder: func(data []byte, v any) error {
			return json.Unmarshal(data, v, jsontext.AllowDuplicateNames(true))
		},
	})

	env := config.LoadEnv()

	database.InitializeConnections(env)

	if env.ENV != "prod" {
		r.Use(logger.New())
	}
	r.Use(recover.New())

	r.Get("/", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"hello": "world"})
	})
	r.Get("/health", func(c fiber.Ctx) error {
		return c.SendString("OK")
	})

	routes.RegisterParams(r.Group("/params"))
	routes.RegisterDb(r.Group("/db"), env)

	r.Use(func(c fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": consts.ErrNotFound})
	})

	return &App{env: env, router: r}
}
