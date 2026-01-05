package routes

import (
	"bufio"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func RegisterParams(r *gin.RouterGroup) {
	r.GET("/search", handleSearchParams)
	r.GET("/url/:dynamic", handleUrlParams)
	r.GET("/header", handleHeaderParams)
	r.POST("/body", handleBodyParams)
	r.GET("/cookie", handleCookieParams)
	r.POST("/form", handleFormParams)
	r.POST("/file", handleFileParams)
}

func handleSearchParams(c *gin.Context) {
	q := c.DefaultQuery("q", "none")

	limitStr := c.Query("limit")
	limit := 10
	if n, err := strconv.Atoi(limitStr); err == nil {
		limit = n
	}

	c.JSON(200, gin.H{"search": q, "limit": limit})
}

func handleUrlParams(c *gin.Context) {
	dynamic := c.Param("dynamic")
	c.JSON(200, gin.H{"dynamic": dynamic})
}

func handleHeaderParams(c *gin.Context) {
	header := c.GetHeader("X-Custom-Header")
	if header == "" {
		header = "none"
	}
	c.JSON(200, gin.H{"header": header})
}

type BodyParams struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}

func handleBodyParams(c *gin.Context) {
	var body BodyParams
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "Invalid request body"})
		return
	}
	c.JSON(200, gin.H{"body": body})
}

func handleCookieParams(c *gin.Context) {
	cookie, err := c.Cookie("foo")
	if err != nil {
		cookie = "none"
	}

	c.SetCookie("bar", "12345", 10, "/", "", false, true)

	c.JSON(200, gin.H{"cookie": cookie})
}

func handleFormParams(c *gin.Context) {
	name := c.DefaultPostForm("name", "none")

	ageStr := c.PostForm("age")
	age := 0
	if n, err := strconv.Atoi(ageStr); err == nil {
		age = n
	}

	c.JSON(200, gin.H{"name": name, "age": age})
}

const (
	maxFileBytes = 1 << 20 // 1MB
	sniffLen     = 512
	nullByte     = 0x00
)

func handleFileParams(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "file not found in form data"})
		return
	}
	if fileHeader.Size > maxFileBytes {
		c.JSON(400, gin.H{"error": "file size exceeds limit"})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(500, gin.H{"error": "unable to open uploaded file"})
		return
	}
	defer file.Close()

	br := bufio.NewReader(file)

	head, err := br.Peek(sniffLen)
	if err != nil && err != io.EOF {
		c.JSON(500, gin.H{"error": "unable to read uploaded file"})
		return
	}

	if mime := http.DetectContentType(head); !strings.HasPrefix(mime, "text/plain") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only text/plain files are allowed"})
		return
	}

	if slices.Contains(head, nullByte) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file does not look like plain text"})
		return
	}

	limited := io.LimitReader(br, maxFileBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "unable to read file content"})
		return
	}
	if int64(len(data)) > maxFileBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file size exceeds limit"})
		return
	}
	if slices.Contains(data, nullByte) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file does not look like plain text"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"filename": fileHeader.Filename,
		"size":     fileHeader.Size,
		"content":  string(data),
	})
}
