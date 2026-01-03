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
	r.Get("/url/{dynamic}", handleUrlParams)
	r.Get("/header", handleHeaderParams)
	r.Post("/body", handleBodyParams)
	r.Get("/cookie", handleCookieParams)

	return r
}

func handleSearchParams(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	qStr := query.Get("q")
	q := cmp.Or(qStr, "none")

	limitStr := query.Get("limit")
	limit := 10
	if n, err := strconv.Atoi(limitStr); err == nil {
		limit = n
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.MarshalWrite(w, map[string]any{"search": q, "limit": limit})
}

func handleUrlParams(w http.ResponseWriter, r *http.Request) {
	dynamic := chi.URLParam(r, "dynamic")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.MarshalWrite(w, map[string]any{"dynamic": dynamic})
}

func handleHeaderParams(w http.ResponseWriter, r *http.Request) {
	headerStr := r.Header.Get("X-Custom-Header")

	header := cmp.Or(headerStr, "none")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.MarshalWrite(w, map[string]any{"header": header})
}

type BodyParams struct {
	Name    string `json:"name"`
	Age     int    `json:"age"`
	IsAdmin bool   `json:"is_admin"`
}

func handleBodyParams(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var body BodyParams
	if err := json.UnmarshalRead(r.Body, &body); err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.MarshalWrite(w, map[string]any{"body": body})
}

func handleCookieParams(w http.ResponseWriter, r *http.Request) {
	cookieStr, err := r.Cookie("foo")

	cookie := "none"
	if err == nil {
		cookie = cookieStr.Value
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "bar",
		Value:    "12345",
		MaxAge:   10,
		HttpOnly: true,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.MarshalWrite(w, map[string]any{"cookie": cookie})
}
