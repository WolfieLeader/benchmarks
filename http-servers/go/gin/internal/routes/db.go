package routes

import (
	"net/http"

	"gin-server/internal/config"
	"gin-server/internal/consts"
	"gin-server/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

const repositoryKey = "repository"

func withRepository(env *config.Env) gin.HandlerFunc {
	return func(c *gin.Context) {
		dbType := c.Param("database")
		repo := database.ResolveRepository(dbType, env)
		if repo == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": consts.ErrNotFound})
			c.Abort()
			return
		}
		c.Set(repositoryKey, repo)
		c.Next()
	}
}

func getRepository(c *gin.Context) database.UserRepository {
	repo, ok := c.MustGet(repositoryKey).(database.UserRepository)
	if !ok {
		panic("repository not found in context - middleware not applied")
	}
	return repo
}

func RegisterDb(r *gin.RouterGroup, env *config.Env) {
	db := r.Group("/:database")
	db.Use(withRepository(env))
	db.POST("/users", createUser)
	db.GET("/users/:id", getUser)
	db.PATCH("/users/:id", updateUser)
	db.DELETE("/users/:id", deleteUser)
	db.DELETE("/users", deleteAllUsers)
	db.GET("/health", healthCheck)
}

func createUser(c *gin.Context) {
	repo := getRepository(c)

	var data database.CreateUser
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidJSON})
		return
	}

	if err := validate.Struct(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidJSON})
		return
	}

	user, err := repo.Create(&data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": consts.ErrInternal})
		return
	}

	c.JSON(http.StatusCreated, user)
}

func getUser(c *gin.Context) {
	repo := getRepository(c)

	id := c.Param("id")
	user, err := repo.FindById(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": consts.ErrInternal})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": consts.ErrNotFound})
		return
	}

	c.JSON(http.StatusOK, user)
}

func updateUser(c *gin.Context) {
	repo := getRepository(c)

	var data database.UpdateUser
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidJSON})
		return
	}

	if err := validate.Struct(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidJSON})
		return
	}

	id := c.Param("id")
	user, err := repo.Update(id, &data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": consts.ErrInternal})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": consts.ErrNotFound})
		return
	}

	c.JSON(http.StatusOK, user)
}

func deleteUser(c *gin.Context) {
	repo := getRepository(c)

	id := c.Param("id")
	deleted, err := repo.Delete(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": consts.ErrInternal})
		return
	}
	if !deleted {
		c.JSON(http.StatusNotFound, gin.H{"error": consts.ErrNotFound})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func deleteAllUsers(c *gin.Context) {
	repo := getRepository(c)

	if err := repo.DeleteAll(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": consts.ErrInternal})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func healthCheck(c *gin.Context) {
	repo := getRepository(c)

	healthy, err := repo.HealthCheck()
	if err != nil || !healthy {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database unavailable"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}
