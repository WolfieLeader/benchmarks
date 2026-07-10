package app

import (
	"log"
	"net/http"
	"shared/consts"
	"time"

	"stdlib-server/internal/utils"
)

// statusRecorder captures the response status so the dev logger can report it.
// Status defaults to 200 (an implicit WriteHeader on the first Write).
type statusRecorder struct {
	http.ResponseWriter

	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// logger prints one line per request (method, path, status, duration). It is
// wired only when ENV != prod (logger off in prod, like every other server).
func logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		//nolint:gosec // G706: dev-only request logger (wired only when ENV != prod); logs method/path for local debugging, the stdlib equivalent of the framework loggers the other servers use
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start))
	})
}

// recoverer turns a handler panic into a 500 JSON error instead of a dropped
// connection — parity with the other servers' recover middleware.
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v", rec)
				utils.WriteError(w, http.StatusInternalServerError, consts.ErrInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
