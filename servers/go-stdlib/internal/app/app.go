package app

import (
	"net/http"
	"shared/config"
	"shared/consts"
	"shared/database"

	"stdlib-server/internal/routes"
	"stdlib-server/internal/utils"
)

type App struct {
	env     *config.Env
	handler http.Handler
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
	mux := http.NewServeMux()

	env := config.LoadEnv(21001)

	database.InitializeConnections(env)

	// "GET /{$}" matches ONLY the exact root path; a bare "GET /" would be a
	// subtree catch-all and swallow every unmatched GET.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"hello":"world"}`))
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	routes.RegisterParams(mux)
	routes.RegisterDb(mux, env)
	routes.RegisterWeb(mux, env.JwtSecret)

	// Catch-all: any path not matched by a more specific pattern renders the
	// suite's {"error":"not found"} shape (stdlib's default 404 is plain text).
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		utils.WriteError(w, http.StatusNotFound, consts.ErrNotFound)
	})

	// Middleware chain (outermost first): recover panics → dev logger (off in
	// prod) → global body cap → router. Plain http.Handler wrappers, no deps.
	var handler http.Handler = mux
	handler = maxBodyBytes(handler)
	if env.ENV != "prod" {
		handler = logger(handler)
	}
	handler = recoverer(handler)

	return &App{env: env, handler: handler}
}
