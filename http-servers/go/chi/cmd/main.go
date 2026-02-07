package main

import (
	"log"

	application "chi-server/internal/app"
)

func main() {
	app := application.New()
	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
