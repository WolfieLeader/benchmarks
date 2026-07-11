package app

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"shared/config"
	"shared/consts"
	"shared/database"

	"fiber-server/internal/routes"
	"fiber-server/internal/utils"

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
		BodyLimit: consts.MaxRequestBytes,
		// fasthttp rejects an over-BodyLimit body before any handler runs and
		// fiber surfaces it here as ErrRequestEntityTooLarge; render it in the
		// suite's error shape instead of fiber's default text/plain response.
		// Everything else keeps the default behavior.
		ErrorHandler: func(c fiber.Ctx, err error) error {
			if errors.Is(err, fiber.ErrRequestEntityTooLarge) {
				return utils.WriteError(c, fiber.StatusRequestEntityTooLarge, consts.ErrRequestTooLarge)
			}
			return fiber.DefaultErrorHandler(c, err)
		},
		JSONEncoder: func(v any) ([]byte, error) { return json.Marshal(v) },
		JSONDecoder: func(data []byte, v any) error {
			return json.Unmarshal(data, v, jsontext.AllowDuplicateNames(true))
		},
	})

	env := config.LoadEnv(5003)

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
	routes.RegisterWeb(r, env.JwtSecret)

	r.Use(func(c fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": consts.ErrNotFound})
	})

	return &App{env: env, router: r}
}
