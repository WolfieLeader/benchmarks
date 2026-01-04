package main

import (
	apppkg "fiber-server/internals/app"
)

func main() {
	app := apppkg.New()
	app.LoadEnv()
	app.Start()
}
