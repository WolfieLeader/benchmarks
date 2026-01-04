package main

import (
	apppkg "chi-server/internals/app"
)

func main() {
	app := apppkg.New()
	app.LoadEnv()
	app.Start()
}
