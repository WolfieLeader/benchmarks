package app

import (
	"chi-server/internal/config"
	"chi-server/internal/consts"
	"chi-server/internal/routes"
	"chi-server/internal/utils"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type App struct {
	env    *config.Env
	router *chi.Mux
}

func New() *App {
	r := chi.NewRouter()

	env := config.LoadEnv()

	if env.ENV != "prod" {
		r.Use(middleware.Logger)
	}
	r.Use(middleware.Recoverer)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message": "Hello, World!"}`))
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	r.Route("/params", func(r chi.Router) { routes.RegisterParams(r) })

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		utils.WriteError(w, http.StatusNotFound, consts.ErrNotFound)
	})

	return &App{env: env, router: r}
}
