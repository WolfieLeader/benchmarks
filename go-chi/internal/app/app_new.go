package app

import (
	"chi-server/internal/routes"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type App struct {
	router *chi.Mux
}

func New() *App {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Mount("/", routes.Root())
	r.Mount("/params", routes.Params())

	return &App{router: r}
}
