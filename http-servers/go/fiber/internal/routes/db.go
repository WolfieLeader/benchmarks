package routes

import (
	"fiber-server/internal/config"
	"fiber-server/internal/database"

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
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
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
	db.Get("/health", healthCheck)
}

func createUser(c *fiber.Ctx) error {
	repo := getRepository(c)

	var data database.CreateUser
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}

	if err := validate.Struct(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}

	user, err := repo.Create(&data)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}

	return c.Status(fiber.StatusCreated).JSON(user)
}

func getUser(c *fiber.Ctx) error {
	repo := getRepository(c)

	id := c.Params("id")
	user, err := repo.FindById(id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}
	if user == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}

	return c.JSON(user)
}

func updateUser(c *fiber.Ctx) error {
	repo := getRepository(c)

	var data database.UpdateUser
	if err := c.BodyParser(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}

	if err := validate.Struct(&data); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}

	id := c.Params("id")
	user, err := repo.Update(id, &data)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}
	if user == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}

	return c.JSON(user)
}

func deleteUser(c *fiber.Ctx) error {
	repo := getRepository(c)

	id := c.Params("id")
	deleted, err := repo.Delete(id)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}
	if !deleted {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}

	return c.JSON(fiber.Map{"success": true})
}

func deleteAllUsers(c *fiber.Ctx) error {
	repo := getRepository(c)

	if err := repo.DeleteAll(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(fiber.Map{"success": true})
}

func resetDatabase(c *fiber.Ctx) error {
	repo := getRepository(c)

	if err := repo.DeleteAll(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}

	return c.JSON(fiber.Map{"status": "ok"})
}

func healthCheck(c *fiber.Ctx) error {
	repo := getRepository(c)

	healthy, err := repo.HealthCheck()
	if err != nil || !healthy {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "database unavailable"})
	}

	return c.JSON(fiber.Map{"status": "healthy"})
}
