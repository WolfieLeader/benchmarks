package params

import (
	"github.com/go-chi/chi/v5"
)

func Router() *chi.Mux {
	r := chi.NewRouter()

	r.Get("/search", handleSearchParams)
	r.Get("/url/{dynamic}", handleUrlParams)
	r.Get("/header", handleHeaderParams)
	r.Post("/body", handleBodyParams)
	r.Get("/cookie", handleCookieParams)
	r.Post("/form", handleFormParams)
	r.Post("/file", handleFileParams)

	return r
}
