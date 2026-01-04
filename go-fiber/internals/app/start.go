package app

import "fmt"

func (app *App) Start() {
	app.router.Listen(fmt.Sprintf("%s:%d", app.env.HOST, app.env.PORT))
}
