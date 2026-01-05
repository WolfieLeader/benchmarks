package main

import (
	application "fiber-server/internal/app"
)

func main() {
	app := application.New()
	app.LoadEnv()
	app.Start()
}
