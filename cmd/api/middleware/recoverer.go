package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
)

// Recoverer catches panics from downstream handlers, logs the panic with its
// stack trace, and returns an internal server error when no response has been
// written yet.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responseWriter := chiMiddleware.NewWrapResponseWriter(w, r.ProtoMajor)

		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}

			if recovered == http.ErrAbortHandler {
				panic(recovered)
			}

			slog.ErrorContext(
				r.Context(),
				"panic recovered",
				"panic", recovered,
				"method", r.Method,
				"path", r.URL.Path,
				"stack", string(debug.Stack()),
			)

			if responseWriter.Status() == 0 {
				http.Error(responseWriter, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(responseWriter, r)
	})
}
