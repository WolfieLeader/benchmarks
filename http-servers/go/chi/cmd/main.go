package main

import (
	application "chi-server/internal/app"
)

func main() {
	app := application.New()
	app.Start()
}
