package routes

import (
	"fiber-server/internal/config"
	"fiber-server/internal/consts"
	"fiber-server/internal/database"
	"fiber-server/internal/utils"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
)

var validate = validator.New()

const repositoryKey = "repository"

func withRepository(env *config.Env) fiber.Handler {
	return func(c *fiber.Ctx) error {
		dbType := c.Params("database")
		repo := database.ResolveRepository(dbType, env)
		if repo == nil {
			return utils.WriteError(c, fiber.StatusNotFound, consts.ErrNotFound, "unknown database type: "+dbType)
		}
		c.Locals(repositoryKey, repo)
		return c.Next()
	}
}

func getRepository(c *fiber.Ctx) database.UserRepository {
	repo, ok := c.Locals(repositoryKey).(database.UserRepository)
	if !ok {
		panic("repository not found in context - middleware not applied")
	}
	return repo
}

func RegisterDb(r fiber.Router, env *config.Env) {
	db := r.Group("/:database", withRepository(env))

	db.Post("/users", createUser)
	db.Get("/users/:id", getUser)
	db.Patch("/users/:id", updateUser)
	db.Delete("/users/:id", deleteUser)
	db.Delete("/users", deleteAllUsers)
	db.Delete("/reset", resetDatabase)
}

func createUser(c *fiber.Ctx) error {
	repo := getRepository(c)

	var data database.CreateUser
	if err := c.BodyParser(&data); err != nil {
		return utils.WriteError(c, fiber.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
	}

	if err := validate.Struct(&data); err != nil {
		return utils.WriteError(c, fiber.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
	}

	user, err := repo.Create(c.UserContext(), &data)
	if err != nil {
		return utils.WriteError(c, fiber.StatusInternalServerError, consts.ErrInternal, err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(user)
}

func getUser(c *fiber.Ctx) error {
	repo := getRepository(c)

	id := c.Params("id")
	user, err := repo.FindById(c.UserContext(), id)
	if err != nil {
		return utils.WriteError(c, fiber.StatusInternalServerError, consts.ErrInternal, err.Error())
	}
	if user == nil {
		return utils.WriteError(c, fiber.StatusNotFound, consts.ErrNotFound, "user with id "+id+" not found")
	}

	return c.JSON(user)
}

func updateUser(c *fiber.Ctx) error {
	repo := getRepository(c)

	var data database.UpdateUser
	if err := c.BodyParser(&data); err != nil {
		return utils.WriteError(c, fiber.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
	}

	if err := validate.Struct(&data); err != nil {
		return utils.WriteError(c, fiber.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
	}

	id := c.Params("id")
	user, err := repo.Update(c.UserContext(), id, &data)
	if err != nil {
		return utils.WriteError(c, fiber.StatusInternalServerError, consts.ErrInternal, err.Error())
	}
	if user == nil {
		return utils.WriteError(c, fiber.StatusNotFound, consts.ErrNotFound, "user with id "+id+" not found")
	}

	return c.JSON(user)
}

func deleteUser(c *fiber.Ctx) error {
	repo := getRepository(c)

	id := c.Params("id")
	deleted, err := repo.Delete(c.UserContext(), id)
	if err != nil {
		return utils.WriteError(c, fiber.StatusInternalServerError, consts.ErrInternal, err.Error())
	}
	if !deleted {
		return utils.WriteError(c, fiber.StatusNotFound, consts.ErrNotFound, "user with id "+id+" not found")
	}

	return c.JSON(fiber.Map{"success": true})
}

func deleteAllUsers(c *fiber.Ctx) error {
	repo := getRepository(c)

	if err := repo.DeleteAll(c.UserContext()); err != nil {
		return utils.WriteError(c, fiber.StatusInternalServerError, consts.ErrInternal, err.Error())
	}

	return c.JSON(fiber.Map{"success": true})
}

func resetDatabase(c *fiber.Ctx) error {
	repo := getRepository(c)

	if err := repo.DeleteAll(c.UserContext()); err != nil {
		return utils.WriteError(c, fiber.StatusInternalServerError, consts.ErrInternal, err.Error())
	}

	return c.JSON(fiber.Map{"status": "ok"})
}
