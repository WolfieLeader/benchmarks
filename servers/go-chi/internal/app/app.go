package app

import (
	"net/http"
	"shared/config"
	"shared/consts"
	"shared/database"

	"chi-server/internal/routes"
	"chi-server/internal/utils"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type App struct {
	env    *config.Env
	router *chi.Mux
}

// maxBodyBytes caps every request body at consts.MaxRequestBytes so no route can
// read an unbounded body. The file route enforces its own smaller 1MB limit; a
// body under the global cap still reaches that check and returns its own 413.
func maxBodyBytes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, consts.MaxRequestBytes)
		next.ServeHTTP(w, r)
	})
}

func New() *App {
	r := chi.NewRouter()

	env := config.LoadEnv(5001)

	database.InitializeConnections(env)

	if env.ENV != "prod" {
		r.Use(middleware.Logger)
	}
	r.Use(middleware.Recoverer)
	r.Use(maxBodyBytes)

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

	r.Route("/params", routes.RegisterParams)
	r.Route("/db", func(r chi.Router) { routes.RegisterDb(r, env) })

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		utils.WriteError(w, http.StatusNotFound, consts.ErrNotFound)
	})

	return &App{env: env, router: r}
}
