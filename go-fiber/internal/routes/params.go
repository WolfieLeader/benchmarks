package routes

import (
	"bufio"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

const (
	maxFileBytes = 1 << 20 // 1MB
	sniffLen     = 512
	nullByte     = 0x00
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
	q := c.Query("q", "none")
	limit := c.QueryInt("limit", 10)
	return c.JSON(fiber.Map{"search": q, "limit": limit})
}

func handleUrlParams(c *fiber.Ctx) error {
	dynamic := c.Params("dynamic")
	return c.JSON(fiber.Map{"dynamic": dynamic})
}

func handleHeaderParams(c *fiber.Ctx) error {
	header := c.Get("X-Custom-Header", "none")
	return c.JSON(fiber.Map{"header": header})
}

func handleBodyParams(c *fiber.Ctx) error {
	var body map[string]any
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	return c.JSON(fiber.Map{"body": body})
}

func handleCookieParams(c *fiber.Ctx) error {
	cookie := c.Cookies("foo", "none")
	c.Cookie(&fiber.Cookie{Name: "bar", Value: "12345", MaxAge: 10, HTTPOnly: true, Path: "/"})
	return c.JSON(fiber.Map{"cookie": cookie})
}

func handleFormParams(c *fiber.Ctx) error {
	name := c.FormValue("name", "none")

	ageStr := c.FormValue("age")
	age := 0
	if n, err := strconv.Atoi(ageStr); err == nil {
		age = n
	}

	return c.JSON(fiber.Map{"name": name, "age": age})
}

func handleFileParams(c *fiber.Ctx) error {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file not found in form data"})
	}
	if fileHeader.Size > maxFileBytes {
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{"error": "file size exceeds limit"})
	}

	file, err := fileHeader.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "unable to open uploaded file"})
	}
	defer file.Close()

	br := bufio.NewReader(file)

	head, err := br.Peek(sniffLen)
	if err != nil && err != io.EOF {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "unable to read file"})
	}

	if mime := http.DetectContentType(head); !strings.HasPrefix(mime, "text/plain") {
		return c.Status(fiber.StatusUnsupportedMediaType).JSON(fiber.Map{"error": "only text/plain files are allowed"})
	}

	if slices.Contains(head, nullByte) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file does not look like plain text"})
	}

	limited := io.LimitReader(br, maxFileBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "unable to read file content"})
	}
	if int64(len(data)) > maxFileBytes {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file size exceeds limit"})
	}
	if slices.Contains(data, nullByte) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file does not look like plain text"})
	}

	return c.JSON(fiber.Map{
		"filename": fileHeader.Filename,
		"size":     fileHeader.Size,
		"content":  string(data),
	})
}
