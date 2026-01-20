package main

import (
	application "gin-server/internal/app"
)

func main() {
	app := application.New()
	app.Start()
}
