package routes

import (
	"bufio"
	"cmp"
	"errors"
	"io"
	"net/http"
	"shared/consts"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"echo-server/internal/utils"

	"github.com/labstack/echo/v4"
)

func RegisterParams(g *echo.Group) {
	g.GET("/search", handleSearchParams)
	g.GET("/url/:dynamic", handleUrlParams)
	g.GET("/header", handleHeaderParams)
	g.POST("/body", handleBodyParams)
	g.GET("/cookie", handleCookieParams)
	g.POST("/form", handleFormParams)
	g.POST("/file", handleFileParams)
}

func handleSearchParams(c echo.Context) error {
	q := cmp.Or(strings.TrimSpace(c.QueryParam("q")), "none")

	limit := consts.DefaultLimit
	limitStr := c.QueryParam("limit")
	if limitStr != "" && !strings.Contains(limitStr, ".") {
		if n, err := strconv.ParseInt(limitStr, 10, 64); err == nil && n >= -(1<<53-1) && n <= (1<<53-1) {
			limit = int(n)
		}
	}

	return c.JSON(http.StatusOK, map[string]any{"search": q, "limit": limit})
}

func handleUrlParams(c echo.Context) error {
	dynamic := c.Param("dynamic")
	return c.JSON(http.StatusOK, map[string]any{"dynamic": dynamic})
}

func handleHeaderParams(c echo.Context) error {
	header := cmp.Or(strings.TrimSpace(c.Request().Header.Get("X-Custom-Header")), "none")
	return c.JSON(http.StatusOK, map[string]any{"header": header})
}

func handleBodyParams(c echo.Context) error {
	var body map[string]any
	if err := utils.BindJSON(c, &body); err != nil {
		return utils.WriteBodyError(c, err)
	}
	if body == nil {
		// JSON null decodes into a nil map without error; reject it like any other
		// non-object body.
		return utils.WriteError(c, http.StatusBadRequest, consts.ErrInvalidJSON, "expected a JSON object")
	}

	return c.JSON(http.StatusOK, map[string]any{"body": body})
}

func handleCookieParams(c echo.Context) error {
	cookie := "none"
	if ck, err := c.Cookie("foo"); err == nil {
		if trimmed := strings.TrimSpace(ck.Value); trimmed != "" {
			cookie = trimmed
		}
	}

	//nolint:gosec // G124: benchmark fixture cookie on a local HTTP-only rig; Secure/SameSite deliberately omitted for response parity across all server implementations
	c.SetCookie(&http.Cookie{Name: "bar", Value: "12345", MaxAge: 10, HttpOnly: true, Path: "/"})
	return c.JSON(http.StatusOK, map[string]any{"cookie": cookie})
}

func handleFormParams(c echo.Context) error {
	contentType := strings.ToLower(c.Request().Header.Get("Content-Type"))
	if !strings.HasPrefix(contentType, "application/x-www-form-urlencoded") && !strings.HasPrefix(contentType, "multipart/form-data") {
		return utils.WriteError(c, http.StatusBadRequest, consts.ErrInvalidForm, consts.ErrExpectedFormContentType)
	}

	name := cmp.Or(strings.TrimSpace(c.FormValue("name")), "none")

	age := 0
	ageStr := strings.TrimSpace(c.FormValue("age"))
	if ageStr != "" {
		if n, err := strconv.ParseInt(ageStr, 10, 64); err == nil && n >= -(1<<53-1) && n <= (1<<53-1) {
			age = int(n)
		}
	}

	return c.JSON(http.StatusOK, map[string]any{"name": name, "age": age})
}

func handleFileParams(c echo.Context) error {
	contentType := strings.ToLower(c.Request().Header.Get("Content-Type"))
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		return utils.WriteError(c, http.StatusBadRequest, consts.ErrInvalidMultipart, consts.ErrExpectedMultipartContentType)
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		return utils.WriteError(c, http.StatusBadRequest, consts.ErrFileNotFound, err.Error())
	}
	if fileHeader.Size > consts.MaxFileBytes {
		return utils.WriteError(c, http.StatusRequestEntityTooLarge, consts.ErrFileSizeExceeded)
	}

	file, err := fileHeader.Open()
	if err != nil {
		return utils.WriteError(c, http.StatusBadRequest, consts.ErrInternal, err.Error())
	}
	defer func() { _ = file.Close() }()

	br := bufio.NewReader(file)

	head, err := br.Peek(consts.SniffLen)
	if err != nil && !errors.Is(err, io.EOF) {
		return utils.WriteError(c, http.StatusBadRequest, consts.ErrInternal, err.Error())
	}

	fileContentType := fileHeader.Header.Get("Content-Type")
	if fileContentType != "" {
		if !strings.HasPrefix(strings.ToLower(fileContentType), "text/plain") {
			return utils.WriteError(c, http.StatusUnsupportedMediaType, consts.ErrInvalidFileType)
		}
	} else {
		if mime := http.DetectContentType(head); !strings.HasPrefix(mime, "text/plain") {
			return utils.WriteError(c, http.StatusUnsupportedMediaType, consts.ErrInvalidFileType)
		}
	}

	if slices.Contains(head, consts.NullByte) {
		return utils.WriteError(c, http.StatusUnsupportedMediaType, consts.ErrNotPlainText)
	}

	limited := io.LimitReader(br, consts.MaxFileBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return utils.WriteError(c, http.StatusBadRequest, consts.ErrInternal, err.Error())
	}
	if int64(len(data)) > consts.MaxFileBytes {
		return utils.WriteError(c, http.StatusRequestEntityTooLarge, consts.ErrFileSizeExceeded)
	}
	if slices.Contains(data, consts.NullByte) {
		return utils.WriteError(c, http.StatusUnsupportedMediaType, consts.ErrNotPlainText)
	}
	if !utf8.Valid(data) {
		return utils.WriteError(c, http.StatusUnsupportedMediaType, consts.ErrNotPlainText)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"filename": fileHeader.Filename,
		"size":     len(data),
		"content":  string(data),
	})
}
