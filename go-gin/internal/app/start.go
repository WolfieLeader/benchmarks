package app

import "fmt"

func (app *App) Start() {
	app.router.Run(fmt.Sprintf("%s:%d", app.env.HOST, app.env.PORT))
}
