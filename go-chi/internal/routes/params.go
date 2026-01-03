package routes

import (
	"cmp"
	"encoding/json/v2"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func Params() *chi.Mux {
	r := chi.NewRouter()

	r.Get("/search", handleSearchParams)
	r.Get("/dynamic/{id}", handleDynamicParams)

	return r
}

func handleSearchParams(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query() // Extract query parameters

	qStr := query.Get("q")
	q := cmp.Or(qStr, "default-search")

	limitStr := query.Get("limit")
	limit := 10
	if n, err := strconv.Atoi(limitStr); err == nil {
		limit = n
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.MarshalWrite(w, map[string]any{"search": q, "limit": limit})
}

func handleDynamicParams(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.MarshalWrite(w, map[string]any{"id": id})
}
