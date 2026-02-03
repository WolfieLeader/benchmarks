package routes

import (
	"bufio"
	"cmp"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"fiber-server/internal/consts"
	"fiber-server/internal/utils"

	"github.com/gofiber/fiber/v2"
)

func RegisterParams(r fiber.Router) {
	r.Get("/search", handleSearchParams)
	r.Get("/url/:dynamic", handleUrlParams)
	r.Get("/header", handleHeaderParams)
	r.Post("/body", handleBodyParams)
	r.Get("/cookie", handleCookieParams)
	r.Post("/form", handleFormParams)
	r.Post("/file", handleFileParams)
}

func handleSearchParams(c *fiber.Ctx) error {
	q := cmp.Or(strings.TrimSpace(c.Query("q")), "none")

	limit := consts.DefaultLimit
	limitStr := c.Query("limit")
	if limitStr != "" && !strings.Contains(limitStr, ".") {
		if n, err := strconv.ParseInt(limitStr, 10, 64); err == nil && n >= -(1<<53-1) && n <= (1<<53-1) {
			limit = int(n)
		}
	}

	return utils.WriteResponse(c, fiber.StatusOK, fiber.Map{"search": q, "limit": limit})
}

func handleUrlParams(c *fiber.Ctx) error {
	dynamic := c.Params("dynamic")
	return utils.WriteResponse(c, fiber.StatusOK, fiber.Map{"dynamic": dynamic})
}

func handleHeaderParams(c *fiber.Ctx) error {
	header := cmp.Or(strings.TrimSpace(c.Get("X-Custom-Header")), "none")
	return utils.WriteResponse(c, fiber.StatusOK, fiber.Map{"header": header})
}

func handleBodyParams(c *fiber.Ctx) error {
	var body map[string]any
	if err := c.BodyParser(&body); err != nil {
		return utils.WriteError(c, fiber.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
	}
	return utils.WriteResponse(c, fiber.StatusOK, fiber.Map{"body": body})
}

func handleCookieParams(c *fiber.Ctx) error {
	cookie := cmp.Or(strings.TrimSpace(c.Cookies("foo")), "none")
	c.Cookie(&fiber.Cookie{Name: "bar", Value: "12345", MaxAge: 10, HTTPOnly: true, Path: "/"})
	return utils.WriteResponse(c, fiber.StatusOK, fiber.Map{"cookie": cookie})
}

func handleFormParams(c *fiber.Ctx) error {
	ct := strings.ToLower(c.Get("Content-Type"))
	if !strings.HasPrefix(ct, "application/x-www-form-urlencoded") && !strings.HasPrefix(ct, "multipart/form-data") {
		return utils.WriteError(c, fiber.StatusBadRequest, consts.ErrInvalidForm)
	}

	name := cmp.Or(strings.TrimSpace(c.FormValue("name")), "none")

	age := 0
	ageStr := strings.TrimSpace(c.FormValue("age"))
	if ageStr != "" {
		if n, err := strconv.ParseInt(ageStr, 10, 64); err == nil && n >= -(1<<53-1) && n <= (1<<53-1) {
			age = int(n)
		}
	}

	return utils.WriteResponse(c, fiber.StatusOK, fiber.Map{"name": name, "age": age})
}

func handleFileParams(c *fiber.Ctx) error {
	ct := strings.ToLower(c.Get("Content-Type"))
	if !strings.HasPrefix(ct, "multipart/form-data") {
		return utils.WriteError(c, fiber.StatusBadRequest, consts.ErrInvalidMultipart)
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		return utils.WriteError(c, fiber.StatusBadRequest, consts.ErrFileNotFound, err.Error())
	}
	if fileHeader.Size > consts.MaxFileBytes {
		return utils.WriteError(c, fiber.StatusRequestEntityTooLarge, consts.ErrFileSizeExceeded)
	}

	file, err := fileHeader.Open()
	if err != nil {
		return utils.WriteError(c, fiber.StatusBadRequest, consts.ErrInternal, err.Error())
	}
	defer func() { _ = file.Close() }()

	br := bufio.NewReader(file)

	head, err := br.Peek(consts.SniffLen)
	if err != nil && !errors.Is(err, io.EOF) {
		return utils.WriteError(c, fiber.StatusBadRequest, consts.ErrInternal, err.Error())
	}

	fileContentType := fileHeader.Header.Get("Content-Type")
	if fileContentType != "" {
		if !strings.HasPrefix(strings.ToLower(fileContentType), "text/plain") {
			return utils.WriteError(c, fiber.StatusUnsupportedMediaType, consts.ErrInvalidFileType)
		}
	} else {
		if mime := http.DetectContentType(head); !strings.HasPrefix(mime, "text/plain") {
			return utils.WriteError(c, fiber.StatusUnsupportedMediaType, consts.ErrInvalidFileType)
		}
	}

	if slices.Contains(head, consts.NullByte) {
		return utils.WriteError(c, fiber.StatusUnsupportedMediaType, consts.ErrNotPlainText)
	}

	limited := io.LimitReader(br, consts.MaxFileBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return utils.WriteError(c, fiber.StatusBadRequest, consts.ErrInternal, err.Error())
	}
	if int64(len(data)) > consts.MaxFileBytes {
		return utils.WriteError(c, fiber.StatusRequestEntityTooLarge, consts.ErrFileSizeExceeded)
	}
	if slices.Contains(data, consts.NullByte) {
		return utils.WriteError(c, fiber.StatusUnsupportedMediaType, consts.ErrNotPlainText)
	}
	if !utf8.Valid(data) {
		return utils.WriteError(c, fiber.StatusUnsupportedMediaType, consts.ErrNotPlainText)
	}

	return utils.WriteResponse(c, fiber.StatusOK, fiber.Map{
		"filename": fileHeader.Filename,
		"size":     len(data),
		"content":  string(data),
	})
}
