package middleware

import (
	"net/http"

	"github.com/CORTA-11/core-api/internal/httpx"
)

// Recoverer catches panics from downstream handlers, logs the panic with its
// stack trace, and returns an internal server error when no response has been
// written yet.
func Recoverer(next http.Handler) http.Handler {
	return httpx.Recover(next)
}
