package app

import (
	"net/http"

	"chi-server/internal/config"
	"chi-server/internal/consts"
	"chi-server/internal/database"
	"chi-server/internal/routes"
	"chi-server/internal/utils"

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

	database.InitializeConnections(env)

	if env.ENV != "prod" {
		r.Use(middleware.Logger)
	}
	r.Use(middleware.Recoverer)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"hello":"world"}`))
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	r.Route("/params", func(r chi.Router) { routes.RegisterParams(r) })
	r.Route("/db", func(r chi.Router) { routes.RegisterDb(r, env) })

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		utils.WriteError(w, http.StatusNotFound, consts.ErrNotFound)
	})

	return &App{env: env, router: r}
}
