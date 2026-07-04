package routes

import (
	"net/http"
	"shared/config"
	"shared/consts"
	"shared/database"

	"echo-server/internal/utils"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
)

var validate = validator.New()

const repositoryKey = "repository"

func withRepository(env *config.Env) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			dbType := c.Param("database")
			repo := database.ResolveRepository(dbType, env)
			if repo == nil {
				return utils.WriteError(c, http.StatusNotFound, consts.ErrNotFound, "unknown database type: "+dbType)
			}
			c.Set(repositoryKey, repo)
			return next(c)
		}
	}
}

func getRepository(c echo.Context) database.UserRepository {
	repo, ok := c.Get(repositoryKey).(database.UserRepository)
	if !ok {
		panic("repository not found in context - middleware not applied")
	}
	return repo
}

func RegisterDb(g *echo.Group, env *config.Env) {
	g.GET("/:database/health", healthCheck(env))

	db := g.Group("/:database", withRepository(env))
	db.POST("/users", createUser)
	db.GET("/users/:id", getUser)
	db.PATCH("/users/:id", updateUser)
	db.DELETE("/users/:id", deleteUser)
	db.DELETE("/users", deleteAllUsers)
	db.DELETE("/reset", resetDatabase)
}

func healthCheck(env *config.Env) echo.HandlerFunc {
	return func(c echo.Context) error {
		dbType := c.Param("database")
		repo := database.ResolveRepository(dbType, env)
		if repo == nil {
			return c.String(http.StatusServiceUnavailable, "Service Unavailable")
		}

		healthy, err := repo.HealthCheck(c.Request().Context())
		if err != nil || !healthy {
			return c.String(http.StatusServiceUnavailable, "Service Unavailable")
		}

		return c.String(http.StatusOK, "OK")
	}
}

func createUser(c echo.Context) error {
	repo := getRepository(c)

	var data database.CreateUser
	if err := utils.BindJSON(c, &data); err != nil {
		return utils.WriteBodyError(c, err)
	}

	if err := validate.Struct(&data); err != nil {
		return utils.WriteError(c, http.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
	}

	user, err := repo.Create(c.Request().Context(), &data)
	if err != nil {
		return utils.WriteError(c, http.StatusInternalServerError, consts.ErrInternal, err.Error())
	}

	return c.JSON(http.StatusCreated, user)
}

func getUser(c echo.Context) error {
	repo := getRepository(c)

	id := c.Param("id")
	user, err := repo.FindById(c.Request().Context(), id)
	if err != nil {
		return utils.WriteError(c, http.StatusInternalServerError, consts.ErrInternal, err.Error())
	}
	if user == nil {
		return utils.WriteError(c, http.StatusNotFound, consts.ErrNotFound, "user with id "+id+" not found")
	}

	return c.JSON(http.StatusOK, user)
}

func updateUser(c echo.Context) error {
	repo := getRepository(c)

	var data database.UpdateUser
	if err := utils.BindJSON(c, &data); err != nil {
		return utils.WriteBodyError(c, err)
	}

	if err := validate.Struct(&data); err != nil {
		return utils.WriteError(c, http.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
	}

	id := c.Param("id")
	user, err := repo.Update(c.Request().Context(), id, &data)
	if err != nil {
		return utils.WriteError(c, http.StatusInternalServerError, consts.ErrInternal, err.Error())
	}
	if user == nil {
		return utils.WriteError(c, http.StatusNotFound, consts.ErrNotFound, "user with id "+id+" not found")
	}

	return c.JSON(http.StatusOK, user)
}

func deleteUser(c echo.Context) error {
	repo := getRepository(c)

	id := c.Param("id")
	deleted, err := repo.Delete(c.Request().Context(), id)
	if err != nil {
		return utils.WriteError(c, http.StatusInternalServerError, consts.ErrInternal, err.Error())
	}
	if !deleted {
		return utils.WriteError(c, http.StatusNotFound, consts.ErrNotFound, "user with id "+id+" not found")
	}

	return c.JSON(http.StatusOK, map[string]bool{"success": true})
}

func deleteAllUsers(c echo.Context) error {
	repo := getRepository(c)

	if err := repo.DeleteAll(c.Request().Context()); err != nil {
		return utils.WriteError(c, http.StatusInternalServerError, consts.ErrInternal, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]bool{"success": true})
}

func resetDatabase(c echo.Context) error {
	repo := getRepository(c)

	if err := repo.DeleteAll(c.Request().Context()); err != nil {
		return utils.WriteError(c, http.StatusInternalServerError, consts.ErrInternal, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
