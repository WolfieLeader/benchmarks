package main

import (
	apppkg "chi-server/app"
)

func main() {
	app := apppkg.New()
	app.LoadEnv()
	app.Start()
}
