package routes

import (
	"bufio"
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"chi-server/internal/consts"
	"chi-server/internal/utils"

	"github.com/go-chi/chi/v5"
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

	limit := consts.DefaultLimit
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
	defer func() { _ = r.Body.Close() }()

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteError(w, http.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
		return
	}

	utils.WriteResponse(w, http.StatusOK, map[string]any{"body": body})
}

func handleCookieParams(w http.ResponseWriter, r *http.Request) {
	cookieVal, err := r.Cookie("foo")

	cookie := "none"
	if err == nil && cookieVal != nil {
		if trimmed := strings.TrimSpace(cookieVal.Value); trimmed != "" {
			cookie = trimmed
		}
	}

	http.SetCookie(w, &http.Cookie{Name: "bar", Value: "12345", MaxAge: 10, HttpOnly: true, Path: "/"})
	utils.WriteResponse(w, http.StatusOK, map[string]any{"cookie": cookie})
}

func handleFormParams(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()

	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if !strings.HasPrefix(contentType, "application/x-www-form-urlencoded") && !strings.HasPrefix(contentType, "multipart/form-data") {
		utils.WriteError(w, http.StatusBadRequest, consts.ErrInvalidForm, consts.ErrExpectedFormContentType)
		return
	}

	if err := r.ParseForm(); err != nil {
		utils.WriteError(w, http.StatusBadRequest, consts.ErrInvalidForm, err.Error())
		return
	}

	name := cmp.Or(strings.TrimSpace(r.FormValue("name")), "none")

	age := 0
	ageStr := strings.TrimSpace(r.FormValue("age"))
	if ageStr != "" {
		if n, err := strconv.ParseInt(ageStr, 10, 64); err == nil && n >= -(1<<53-1) && n <= (1<<53-1) {
			age = int(n)
		}
	}

	utils.WriteResponse(w, http.StatusOK, map[string]any{"name": name, "age": age})
}

func handleFileParams(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()

	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		utils.WriteError(w, http.StatusBadRequest, consts.ErrInvalidMultipart, consts.ErrExpectedMultipartContentType)
		return
	}

	if err := r.ParseMultipartForm(consts.MaxFileBytes); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "too large") {
			utils.WriteError(w, http.StatusRequestEntityTooLarge, consts.ErrFileSizeExceeded)
			return
		}
		utils.WriteError(w, http.StatusBadRequest, consts.ErrInvalidMultipart, err.Error())
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, consts.ErrFileNotFound, err.Error())
		return
	}
	defer func() { _ = file.Close() }()

	br := bufio.NewReader(file)

	head, err := br.Peek(consts.SniffLen)
	if err != nil && !errors.Is(err, io.EOF) {
		utils.WriteError(w, http.StatusBadRequest, consts.ErrInternal, err.Error())
		return
	}

	fileContentType := fileHeader.Header.Get("Content-Type")
	if fileContentType != "" {
		if !strings.HasPrefix(strings.ToLower(fileContentType), "text/plain") {
			utils.WriteError(w, http.StatusUnsupportedMediaType, consts.ErrInvalidFileType)
			return
		}
	} else {
		if mime := http.DetectContentType(head); !strings.HasPrefix(mime, "text/plain") {
			utils.WriteError(w, http.StatusUnsupportedMediaType, consts.ErrInvalidFileType)
			return
		}
	}

	if slices.Contains(head, consts.NullByte) {
		utils.WriteError(w, http.StatusUnsupportedMediaType, consts.ErrNotPlainText)
		return
	}

	limited := io.LimitReader(br, consts.MaxFileBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		utils.WriteError(w, http.StatusBadRequest, consts.ErrInternal, err.Error())
		return
	}
	if int64(len(data)) > consts.MaxFileBytes {
		utils.WriteError(w, http.StatusRequestEntityTooLarge, consts.ErrFileSizeExceeded)
		return
	}
	if slices.Contains(data, consts.NullByte) {
		utils.WriteError(w, http.StatusUnsupportedMediaType, consts.ErrNotPlainText)
		return
	}
	if !utf8.Valid(data) {
		utils.WriteError(w, http.StatusUnsupportedMediaType, consts.ErrNotPlainText)
		return
	}

	utils.WriteResponse(w, http.StatusOK, map[string]any{
		"filename": fileHeader.Filename,
		"size":     len(data),
		"content":  string(data),
	})
}
