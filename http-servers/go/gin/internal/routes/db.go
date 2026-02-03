package routes

import (
	"net/http"

	"gin-server/internal/config"
	"gin-server/internal/consts"
	"gin-server/internal/database"
	"gin-server/internal/utils"

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
			utils.WriteError(c, http.StatusNotFound, consts.ErrNotFound, "unknown database type: "+dbType)
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
	db.DELETE("/reset", resetDatabase)
}

func createUser(c *gin.Context) {
	repo := getRepository(c)

	var data database.CreateUser
	if err := c.ShouldBindJSON(&data); err != nil {
		utils.WriteError(c, http.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
		return
	}

	if err := validate.Struct(&data); err != nil {
		utils.WriteError(c, http.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
		return
	}

	user, err := repo.Create(c.Request.Context(), &data)
	if err != nil {
		utils.WriteError(c, http.StatusInternalServerError, consts.ErrInternal, err.Error())
		return
	}

	c.JSON(http.StatusCreated, user)
}

func getUser(c *gin.Context) {
	repo := getRepository(c)

	id := c.Param("id")
	user, err := repo.FindById(c.Request.Context(), id)
	if err != nil {
		utils.WriteError(c, http.StatusInternalServerError, consts.ErrInternal, err.Error())
		return
	}
	if user == nil {
		utils.WriteError(c, http.StatusNotFound, consts.ErrNotFound, "user with id "+id+" not found")
		return
	}

	c.JSON(http.StatusOK, user)
}

func updateUser(c *gin.Context) {
	repo := getRepository(c)

	var data database.UpdateUser
	if err := c.ShouldBindJSON(&data); err != nil {
		utils.WriteError(c, http.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
		return
	}

	if err := validate.Struct(&data); err != nil {
		utils.WriteError(c, http.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
		return
	}

	id := c.Param("id")
	user, err := repo.Update(c.Request.Context(), id, &data)
	if err != nil {
		utils.WriteError(c, http.StatusInternalServerError, consts.ErrInternal, err.Error())
		return
	}
	if user == nil {
		utils.WriteError(c, http.StatusNotFound, consts.ErrNotFound, "user with id "+id+" not found")
		return
	}

	c.JSON(http.StatusOK, user)
}

func deleteUser(c *gin.Context) {
	repo := getRepository(c)

	id := c.Param("id")
	deleted, err := repo.Delete(c.Request.Context(), id)
	if err != nil {
		utils.WriteError(c, http.StatusInternalServerError, consts.ErrInternal, err.Error())
		return
	}
	if !deleted {
		utils.WriteError(c, http.StatusNotFound, consts.ErrNotFound, "user with id "+id+" not found")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func deleteAllUsers(c *gin.Context) {
	repo := getRepository(c)

	if err := repo.DeleteAll(c.Request.Context()); err != nil {
		utils.WriteError(c, http.StatusInternalServerError, consts.ErrInternal, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func resetDatabase(c *gin.Context) {
	repo := getRepository(c)

	if err := repo.DeleteAll(c.Request.Context()); err != nil {
		utils.WriteError(c, http.StatusInternalServerError, consts.ErrInternal, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
