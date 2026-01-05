package app

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Env struct {
	HOST string
	PORT uint16
}

const (
	defaultHost = "localhost"
	defaultPort = 5000
)

func (app *App) LoadEnv() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	app.env = &Env{
		HOST: defaultHost,
		PORT: defaultPort,
	}

	if host, ok := os.LookupEnv("HOST"); ok {
		app.env.HOST = host
	}

	if n, err := strconv.ParseUint(os.Getenv("PORT"), 10, 16); err == nil {
		app.env.PORT = uint16(n)
	}
}
