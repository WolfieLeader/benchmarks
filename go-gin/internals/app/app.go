package app

import "github.com/gin-gonic/gin"

type App struct {
	env    *Env
	router *gin.Engine
}

func New() *App {
	r := gin.Default()

	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "Hello, World!"})
	})
	r.GET("/ping", func(c *gin.Context) {
		c.String(200, "PONG!")
	})

	return &App{router: r}
}
