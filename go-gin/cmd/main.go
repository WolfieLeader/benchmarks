package main

import (
	apppkg "gin-server/internals/app"
)

func main() {
	app := apppkg.New()
	app.LoadEnv()
	app.Start()
}
