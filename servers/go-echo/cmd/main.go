package main

import (
	"log"

	application "echo-server/internal/app"
)

func main() {
	app := application.New()
	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
