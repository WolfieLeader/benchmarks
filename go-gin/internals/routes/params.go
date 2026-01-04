package routes

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

func RegisterParams(r *gin.RouterGroup) {
	r.GET("/search", handleSearchParams)
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
