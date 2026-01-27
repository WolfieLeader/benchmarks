package app

import (
	"chi-server/internal/config"
	"chi-server/internal/consts"
	"chi-server/internal/routes"
	"chi-server/internal/utils"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type App struct {
	env    *config.Env
	router *chi.Mux
}

func New() *App {
	r := chi.NewRouter()

	env := config.LoadEnv()

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message": "Hello World"}`))
	})

	r.Route("/params", func(r chi.Router) { routes.RegisterParams(r) })

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		utils.WriteError(w, http.StatusNotFound, consts.ErrNotFound)
	})

	return &App{env: env, router: r}
}
