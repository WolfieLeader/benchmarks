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

	"chi-server/internal/utils"

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

	q := cmp.Or(strings.TrimSpace(query.Get("q")), "none")

	limit := 10
	limitStr := query.Get("limit")
	if limitStr != "" && !strings.Contains(limitStr, ".") {
		if n, err := strconv.ParseInt(limitStr, 10, 64); err == nil && n >= -(1<<53-1) && n <= (1<<53-1) {
			limit = int(n)
		}
	}

	utils.WriteResponse(w, http.StatusOK, map[string]any{"search": q, "limit": limit})
}

func handleUrlParams(w http.ResponseWriter, r *http.Request) {
	dynamic := chi.URLParam(r, "dynamic")
	utils.WriteResponse(w, http.StatusOK, map[string]any{"dynamic": dynamic})
}

func handleHeaderParams(w http.ResponseWriter, r *http.Request) {
	header := cmp.Or(strings.TrimSpace(r.Header.Get("X-Custom-Header")), "none")
	utils.WriteResponse(w, http.StatusOK, map[string]any{"header": header})
}

func handleBodyParams(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var body map[string]any
	if err := json.UnmarshalRead(r.Body, &body); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid JSON body", nil)
		return
	}

	utils.WriteResponse(w, http.StatusOK, map[string]any{"body": body})
}

func handleCookieParams(w http.ResponseWriter, r *http.Request) {
	cookieStr, err := r.Cookie("foo")

	cookie := "none"
	if trimmed := strings.TrimSpace(cookieStr.Value); err == nil && trimmed != "" {
		cookie = trimmed
	}

	http.SetCookie(w, &http.Cookie{Name: "bar", Value: "12345", MaxAge: 10, HttpOnly: true, Path: "/"})
	utils.WriteResponse(w, http.StatusOK, map[string]any{"cookie": cookie})
}

func handleFormParams(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if !strings.HasPrefix(contentType, "application/x-www-form-urlencoded") && !strings.HasPrefix(contentType, "multipart/form-data") {
		utils.WriteError(w, http.StatusBadRequest, "invalid form data", nil)
		return
	}

	if err := r.ParseForm(); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid form data", nil)
		return
	}

	name := cmp.Or(strings.TrimSpace(r.FormValue("name")), "none")

	age := 0
	ageStr := strings.TrimSpace(r.FormValue("age"))
	if ageStr != "" && !strings.Contains(ageStr, ".") {
		if n, err := strconv.ParseInt(ageStr, 10, 64); err == nil && n >= -(1<<53-1) && n <= (1<<53-1) {
			age = int(n)
		}
	}

	utils.WriteResponse(w, http.StatusOK, map[string]any{"name": name, "age": age})
}

func handleFileParams(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		utils.WriteError(w, http.StatusBadRequest, "invalid multipart form data", nil)
		return
	}

	if err := r.ParseMultipartForm(maxFileBytes); err != nil {
		utils.WriteError(w, http.StatusBadRequest, "invalid multipart form data", nil)
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "file not found in form data", nil)
		return
	}
	defer file.Close()

	br := bufio.NewReader(file)

	head, err := br.Peek(sniffLen)
	if err != nil && err != io.EOF {
		utils.WriteError(w, http.StatusBadRequest, "unable to read file", nil)
		return
	}

	if mime := http.DetectContentType(head); !strings.HasPrefix(mime, "text/plain") {
		utils.WriteError(w, http.StatusUnsupportedMediaType, "only text/plain files are allowed", nil)
		return
	}

	if slices.Contains(head, nullByte) {
		utils.WriteError(w, http.StatusUnsupportedMediaType, "file does not look like plain text", nil)
		return
	}

	limited := io.LimitReader(br, maxFileBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, "unable to read file content", nil)
		return
	}
	if int64(len(data)) > maxFileBytes {
		utils.WriteError(w, http.StatusRequestEntityTooLarge, "file size exceeds limit", nil)
		return
	}
	if slices.Contains(data, nullByte) {
		utils.WriteError(w, http.StatusUnsupportedMediaType, "file does not look like plain text", nil)
		return
	}
	if !utf8.Valid(data) {
		utils.WriteError(w, http.StatusUnsupportedMediaType, "file does not look like plain text", nil)
		return
	}

	utils.WriteResponse(w, http.StatusOK, map[string]any{
		"filename": fileHeader.Filename,
		"size":     len(data),
		"content":  string(data),
	})
}
