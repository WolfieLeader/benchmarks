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

	"gin-server/internal/consts"
	"gin-server/internal/utils"

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
	q := cmp.Or(strings.TrimSpace(c.Query("q")), "none")

	limit := consts.DefaultLimit
	limitStr := c.Query("limit")
	if limitStr != "" && !strings.Contains(limitStr, ".") {
		if n, err := strconv.ParseInt(limitStr, 10, 64); err == nil && n >= -(1<<53-1) && n <= (1<<53-1) {
			limit = int(n)
		}
	}

	utils.WriteResponse(c, http.StatusOK, gin.H{"search": q, "limit": limit})
}

func handleUrlParams(c *gin.Context) {
	dynamic := c.Param("dynamic")
	utils.WriteResponse(c, http.StatusOK, gin.H{"dynamic": dynamic})
}

func handleHeaderParams(c *gin.Context) {
	header := cmp.Or(strings.TrimSpace(c.GetHeader("X-Custom-Header")), "none")
	utils.WriteResponse(c, http.StatusOK, gin.H{"header": header})
}

func handleBodyParams(c *gin.Context) {
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.WriteError(c, http.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
		return
	}
	utils.WriteResponse(c, http.StatusOK, gin.H{"body": body})
}

func handleCookieParams(c *gin.Context) {
	cookieStr, err := c.Cookie("foo")

	cookie := "none"
	if trimmed := strings.TrimSpace(cookieStr); err == nil && trimmed != "" {
		cookie = trimmed
	}

	c.SetCookie("bar", "12345", 10, "/", "", false, true)
	utils.WriteResponse(c, http.StatusOK, gin.H{"cookie": cookie})
}

func handleFormParams(c *gin.Context) {
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if !strings.HasPrefix(contentType, "application/x-www-form-urlencoded") && !strings.HasPrefix(contentType, "multipart/form-data") {
		utils.WriteError(c, http.StatusBadRequest, consts.ErrInvalidForm, consts.ErrExpectedFormContentType)
		return
	}

	name := cmp.Or(strings.TrimSpace(c.PostForm("name")), "none")

	age := 0
	ageStr := strings.TrimSpace(c.PostForm("age"))
	if ageStr != "" {
		if n, err := strconv.ParseInt(ageStr, 10, 64); err == nil && n >= -(1<<53-1) && n <= (1<<53-1) {
			age = int(n)
		}
	}

	utils.WriteResponse(c, http.StatusOK, gin.H{"name": name, "age": age})
}

func handleFileParams(c *gin.Context) {
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		utils.WriteError(c, http.StatusBadRequest, consts.ErrInvalidMultipart, consts.ErrExpectedMultipartContentType)
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		utils.WriteError(c, http.StatusBadRequest, consts.ErrFileNotFound, err.Error())
		return
	}
	if fileHeader.Size > consts.MaxFileBytes {
		utils.WriteError(c, http.StatusRequestEntityTooLarge, consts.ErrFileSizeExceeded)
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		utils.WriteError(c, http.StatusBadRequest, consts.ErrInternal, err.Error())
		return
	}
	defer func() { _ = file.Close() }()

	br := bufio.NewReader(file)

	head, err := br.Peek(consts.SniffLen)
	if err != nil && !errors.Is(err, io.EOF) {
		utils.WriteError(c, http.StatusBadRequest, consts.ErrInternal, err.Error())
		return
	}

	fileContentType := fileHeader.Header.Get("Content-Type")
	if fileContentType != "" {
		if !strings.HasPrefix(strings.ToLower(fileContentType), "text/plain") {
			utils.WriteError(c, http.StatusUnsupportedMediaType, consts.ErrInvalidFileType)
			return
		}
	} else {
		if mime := http.DetectContentType(head); !strings.HasPrefix(mime, "text/plain") {
			utils.WriteError(c, http.StatusUnsupportedMediaType, consts.ErrInvalidFileType)
			return
		}
	}

	if slices.Contains(head, consts.NullByte) {
		utils.WriteError(c, http.StatusUnsupportedMediaType, consts.ErrNotPlainText)
		return
	}

	limited := io.LimitReader(br, consts.MaxFileBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		utils.WriteError(c, http.StatusBadRequest, consts.ErrInternal, err.Error())
		return
	}
	if int64(len(data)) > consts.MaxFileBytes {
		utils.WriteError(c, http.StatusRequestEntityTooLarge, consts.ErrFileSizeExceeded)
		return
	}
	if slices.Contains(data, consts.NullByte) {
		utils.WriteError(c, http.StatusUnsupportedMediaType, consts.ErrNotPlainText)
		return
	}
	if !utf8.Valid(data) {
		utils.WriteError(c, http.StatusUnsupportedMediaType, consts.ErrNotPlainText)
		return
	}

	utils.WriteResponse(c, http.StatusOK, gin.H{
		"filename": fileHeader.Filename,
		"size":     len(data),
		"content":  string(data),
	})
}
