package routes

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func Root() *chi.Mux {
	r := chi.NewRouter()

	r.Get("/", handleRoot)
	r.Get("/ping", handlePing)

	return r
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"message": "Hello, World!"}`))
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("PONG!"))
}
