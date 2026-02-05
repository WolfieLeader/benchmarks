package app

import (
	"net/http"

	"gin-server/internal/config"
	"gin-server/internal/consts"
	"gin-server/internal/database"
	"gin-server/internal/routes"

	"github.com/gin-gonic/gin"
)

type App struct {
	env    *config.Env
	router *gin.Engine
}

func New() *App {
	r := gin.New()

	env := config.LoadEnv()

	database.InitializeConnections(env)

	if env.ENV != "prod" {
		r.Use(gin.Logger())
	}
	r.Use(gin.Recovery())

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"hello": "world"})
	})
	r.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})
	routes.RegisterParams(r.Group("/params"))
	routes.RegisterDb(r.Group("/db"), env)

	r.NoRoute(func(c *gin.Context) {
		c.JSON(404, gin.H{"error": consts.ErrNotFound})
	})

	return &App{env: env, router: r}
}
