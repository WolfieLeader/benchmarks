package main

import (
	"log"

	application "gin-server/internal/app"
)

func main() {
	app := application.New()
	if err := app.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
