package app

import (
	"chi-server/internal/handlers"

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

	r.Get("/", handlers.HelloWorld)
	r.Get("/ping", handlers.Ping)

	return &App{router: r}
}
