package routes

import (
	"context"
	"encoding/json"
	"net/http"

	"chi-server/internal/config"
	"chi-server/internal/consts"
	"chi-server/internal/database"
	"chi-server/internal/utils"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

type repositoryKey struct{}

func withRepository(env *config.Env) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			dbType := chi.URLParam(r, "database")
			repo := database.ResolveRepository(dbType, env)
			if repo == nil {
				utils.WriteError(w, http.StatusNotFound, consts.ErrNotFound, "unknown database type: "+dbType)
				return
			}
			ctx := context.WithValue(r.Context(), repositoryKey{}, repo)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func getRepository(r *http.Request) database.UserRepository {
	repo, ok := r.Context().Value(repositoryKey{}).(database.UserRepository)
	if !ok {
		panic("repository not found in context - middleware not applied")
	}
	return repo
}

func RegisterDb(r chi.Router, env *config.Env) {
	r.Route("/{database}", func(r chi.Router) {
		r.Use(withRepository(env))
		r.Post("/users", createUser)
		r.Get("/users/{id}", getUser)
		r.Patch("/users/{id}", updateUser)
		r.Delete("/users/{id}", deleteUser)
		r.Delete("/users", deleteAllUsers)
		r.Delete("/reset", resetDatabase)
	})
}

func createUser(w http.ResponseWriter, r *http.Request) {
	repo := getRepository(r)

	var data database.CreateUser
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		utils.WriteError(w, http.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
		return
	}

	if err := validate.Struct(&data); err != nil {
		utils.WriteError(w, http.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
		return
	}

	user, err := repo.Create(r.Context(), &data)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, consts.ErrInternal, err.Error())
		return
	}

	utils.WriteResponse(w, http.StatusCreated, user)
}

func getUser(w http.ResponseWriter, r *http.Request) {
	repo := getRepository(r)

	id := chi.URLParam(r, "id")
	user, err := repo.FindById(r.Context(), id)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, consts.ErrInternal, err.Error())
		return
	}
	if user == nil {
		utils.WriteError(w, http.StatusNotFound, consts.ErrNotFound, "user with id "+id+" not found")
		return
	}

	utils.WriteResponse(w, http.StatusOK, user)
}

func updateUser(w http.ResponseWriter, r *http.Request) {
	repo := getRepository(r)

	var data database.UpdateUser
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		utils.WriteError(w, http.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
		return
	}

	if err := validate.Struct(&data); err != nil {
		utils.WriteError(w, http.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
		return
	}

	id := chi.URLParam(r, "id")
	user, err := repo.Update(r.Context(), id, &data)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, consts.ErrInternal, err.Error())
		return
	}
	if user == nil {
		utils.WriteError(w, http.StatusNotFound, consts.ErrNotFound, "user with id "+id+" not found")
		return
	}

	utils.WriteResponse(w, http.StatusOK, user)
}

func deleteUser(w http.ResponseWriter, r *http.Request) {
	repo := getRepository(r)

	id := chi.URLParam(r, "id")
	deleted, err := repo.Delete(r.Context(), id)
	if err != nil {
		utils.WriteError(w, http.StatusInternalServerError, consts.ErrInternal, err.Error())
		return
	}
	if !deleted {
		utils.WriteError(w, http.StatusNotFound, consts.ErrNotFound, "user with id "+id+" not found")
		return
	}

	utils.WriteResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func deleteAllUsers(w http.ResponseWriter, r *http.Request) {
	repo := getRepository(r)

	if err := repo.DeleteAll(r.Context()); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, consts.ErrInternal, err.Error())
		return
	}

	utils.WriteResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func resetDatabase(w http.ResponseWriter, r *http.Request) {
	repo := getRepository(r)

	if err := repo.DeleteAll(r.Context()); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, consts.ErrInternal, err.Error())
		return
	}

	utils.WriteResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}
