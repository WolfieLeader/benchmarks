package app

import (
	"gin-server/internal/config"
	"gin-server/internal/consts"
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

	if env.ENV != "prod" {
		r.Use(gin.Logger())
	}
	r.Use(gin.Recovery())

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "Hello, World!"})
	})
	r.GET("/health", func(c *gin.Context) {
		c.String(200, "OK")
	})
	routes.RegisterParams(r.Group("/params"))

	r.NoRoute(func(c *gin.Context) {
		c.JSON(404, gin.H{"error": consts.ErrNotFound})
	})

	return &App{env: env, router: r}
}
