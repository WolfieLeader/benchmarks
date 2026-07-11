package app

import (
	"net/http"
	"shared/config"
	"shared/consts"
	"shared/database"

	"gin-server/internal/routes"
	"gin-server/internal/utils"

	"github.com/gin-gonic/gin"
)

type App struct {
	env    *config.Env
	router *gin.Engine
}

// maxBodyBytes caps every request body at consts.MaxRequestBytes so no route can
// read an unbounded body. The file route enforces its own smaller 1MB limit; a
// body under the global cap still reaches that check and returns its own 413.
func maxBodyBytes() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, consts.MaxRequestBytes)
		c.Next()
	}
}

func New() *App {
	r := gin.New()

	env := config.LoadEnv(5002)

	database.InitializeConnections(env)

	if env.ENV != "prod" {
		r.Use(gin.Logger())
	}
	r.Use(gin.Recovery())
	r.Use(maxBodyBytes())

	r.GET("/", func(c *gin.Context) {
		utils.WriteResponse(c, http.StatusOK, gin.H{"hello": "world"})
	})
	r.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})
	routes.RegisterParams(r.Group("/params"))
	routes.RegisterDb(r.Group("/db"), env)
	routes.RegisterWeb(r, env.JwtSecret)

	r.NoRoute(func(c *gin.Context) {
		utils.WriteResponse(c, http.StatusNotFound, gin.H{"error": consts.ErrNotFound})
	})

	return &App{env: env, router: r}
}
