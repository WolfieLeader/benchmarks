package routes

import (
	"encoding/json/v2"
	"net/http"
	"shared/config"
	"shared/consts"
	"shared/database"

	"stdlib-server/internal/utils"

	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

// repoHandler is a db handler that has already been handed its resolved
// repository — withRepo does the {database} lookup so every handler can assume
// a live repo (no per-route middleware in stdlib's mux).
type repoHandler func(w http.ResponseWriter, r *http.Request, repo database.UserRepository)

// withRepo resolves the {database} path value to a repository, 404-ing on an
// unknown database, then invokes the wrapped handler with it.
func withRepo(env *config.Env, h repoHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dbType := r.PathValue("database")
		repo := database.ResolveRepository(dbType, env)
		if repo == nil {
			utils.WriteError(w, http.StatusNotFound, consts.ErrNotFound, "unknown database type: "+dbType)
			return
		}
		h(w, r, repo)
	}
}

func RegisterDb(mux *http.ServeMux, env *config.Env) {
	mux.HandleFunc("GET /db/{database}/health", dbHealth(env))
	mux.HandleFunc("POST /db/{database}/users", withRepo(env, createUser))
	mux.HandleFunc("GET /db/{database}/users/{id}", withRepo(env, getUser))
	mux.HandleFunc("PATCH /db/{database}/users/{id}", withRepo(env, updateUser))
	mux.HandleFunc("DELETE /db/{database}/users/{id}", withRepo(env, deleteUser))
	mux.HandleFunc("DELETE /db/{database}/users", withRepo(env, deleteAllUsers))
	mux.HandleFunc("DELETE /db/{database}/reset", withRepo(env, resetDatabase))
}

func dbHealth(env *config.Env) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")

		dbType := r.PathValue("database")
		repo := database.ResolveRepository(dbType, env)
		if repo == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("Service Unavailable"))
			return
		}

		healthy, err := repo.HealthCheck(r.Context())
		if err != nil || !healthy {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("Service Unavailable"))
			return
		}

		_, _ = w.Write([]byte("OK"))
	}
}

func createUser(w http.ResponseWriter, r *http.Request, repo database.UserRepository) {
	var data database.CreateUser
	if err := json.UnmarshalRead(r.Body, &data, decodeOpts); err != nil {
		utils.WriteBodyError(w, err)
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

func getUser(w http.ResponseWriter, r *http.Request, repo database.UserRepository) {
	id := r.PathValue("id")
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

func updateUser(w http.ResponseWriter, r *http.Request, repo database.UserRepository) {
	var data database.UpdateUser
	if err := json.UnmarshalRead(r.Body, &data, decodeOpts); err != nil {
		utils.WriteBodyError(w, err)
		return
	}

	if err := validate.Struct(&data); err != nil {
		utils.WriteError(w, http.StatusBadRequest, consts.ErrInvalidJSON, err.Error())
		return
	}

	id := r.PathValue("id")
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

func deleteUser(w http.ResponseWriter, r *http.Request, repo database.UserRepository) {
	id := r.PathValue("id")
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

func deleteAllUsers(w http.ResponseWriter, r *http.Request, repo database.UserRepository) {
	if err := repo.DeleteAll(r.Context()); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, consts.ErrInternal, err.Error())
		return
	}

	utils.WriteResponse(w, http.StatusOK, map[string]bool{"success": true})
}

func resetDatabase(w http.ResponseWriter, r *http.Request, repo database.UserRepository) {
	if err := repo.DeleteAll(r.Context()); err != nil {
		utils.WriteError(w, http.StatusInternalServerError, consts.ErrInternal, err.Error())
		return
	}

	utils.WriteResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}
