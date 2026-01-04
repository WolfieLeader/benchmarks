package main

import (
	apppkg "fiber-server/app"
)

func main() {
	app := apppkg.New()

	app.LoadEnv()

	app.Start()
}
