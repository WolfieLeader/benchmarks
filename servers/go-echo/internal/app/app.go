package app

import (
	"net/http"
	"shared/config"
	"shared/consts"
	"shared/database"

	"echo-server/internal/routes"
	"echo-server/internal/utils"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type App struct {
	env  *config.Env
	echo *echo.Echo
}

// maxBodyBytes caps every request body at consts.MaxRequestBytes so no route can read an
// unbounded body. The file route enforces its own smaller 1MB limit; a body under the
// global cap still reaches that check and returns its own 413.
func maxBodyBytes(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, consts.MaxRequestBytes)
		return next(c)
	}
}

func New() *App {
	e := echo.New()
	// echo prints an ASCII banner and the listen address to stdout on start; suppress both
	// so prod output stays clean and the single dev banner (start.go) is the only line.
	e.HideBanner = true
	e.HidePort = true
	// Route the whole server through encoding/json/v2 (c.JSON + body decode), matching the
	// other Go servers instead of echo's stdlib-v1 DefaultJSONSerializer.
	e.JSONSerializer = utils.JSONSerializer{}

	env := config.LoadEnv(5005)

	database.InitializeConnections(env)

	if env.ENV != "prod" {
		e.Use(middleware.Logger())
	}
	e.Use(middleware.Recover())
	e.Use(maxBodyBytes)

	e.GET("/", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]any{"hello": "world"})
	})
	e.GET("/health", func(c echo.Context) error {
		return c.String(http.StatusOK, "OK")
	})

	routes.RegisterParams(e.Group("/params"))
	routes.RegisterDb(e.Group("/db"), env)

	// echo's default 404 renders {"message":"Not Found"}; RouteNotFound is echo's hook to
	// render the suite's {"error": ...} shape for unmatched routes.
	e.RouteNotFound("/*", func(c echo.Context) error {
		return utils.WriteError(c, http.StatusNotFound, consts.ErrNotFound)
	})

	return &App{env: env, echo: e}
}
