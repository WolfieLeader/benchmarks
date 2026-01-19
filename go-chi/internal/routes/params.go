package routes

import (
	"bufio"
	"cmp"
	"encoding/json/v2"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
)

const (
	maxFileBytes = 1 << 20 // 1MB
	sniffLen     = 512
	nullByte     = 0x00
)

func RegisterParams(r chi.Router) {
	r.Get("/search", handleSearchParams)
	r.Get("/url/{dynamic}", handleUrlParams)
	r.Get("/header", handleHeaderParams)
	r.Post("/body", handleBodyParams)
	r.Get("/cookie", handleCookieParams)
	r.Post("/form", handleFormParams)
	r.Post("/file", handleFileParams)
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

func handleBodyParams(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var body map[string]any
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
		Path:     "/",
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.MarshalWrite(w, map[string]any{"cookie": cookie})
}

func handleFormParams(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	err := r.ParseForm()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error": "invalid form data"}`, http.StatusBadRequest)
		return
	}

	nameStr := r.FormValue("name")
	name := cmp.Or(strings.TrimSpace(nameStr), "none")

	ageStr := r.FormValue("age")
	age := 0
	if n, err := strconv.Atoi(ageStr); err == nil {
		age = n
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.MarshalWrite(w, map[string]any{"name": name, "age": age})
}

func handleFileParams(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if err := r.ParseMultipartForm(maxFileBytes); err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"invalid multipart form data"}`, http.StatusRequestEntityTooLarge)
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"file not found in form data"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	br := bufio.NewReader(file)

	head, err := br.Peek(sniffLen)
	if err != nil && err != io.EOF {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"unable to read file"}`, http.StatusBadRequest)
		return
	}

	if mime := http.DetectContentType(head); !strings.HasPrefix(mime, "text/plain") {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"only text/plain files are allowed"}`, http.StatusUnsupportedMediaType)
		return
	}

	if slices.Contains(head, nullByte) {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"file does not look like plain text"}`, http.StatusUnsupportedMediaType)
		return
	}

	limited := io.LimitReader(br, maxFileBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"unable to read file content"}`, http.StatusBadRequest)
		return
	}
	if int64(len(data)) > maxFileBytes {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"file too large"}`, http.StatusRequestEntityTooLarge)
		return
	}
	if slices.Contains(data, nullByte) {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"file does not look like plain text"}`, http.StatusUnsupportedMediaType)
		return
	}
	if !utf8.Valid(data) {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"file does not look like plain text"}`, http.StatusUnsupportedMediaType)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.MarshalWrite(w, map[string]any{
		"filename":  fileHeader.Filename,
		"bytesRead": fileHeader.Size,
		"content":   string(data),
	})
}
