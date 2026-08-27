package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/CORTA-11/core-api/internal/service"
)

type contextKey string

const userIDKey contextKey = "userID"

// JWTMiddleware validates the Authorization header containing a JWT bearer token.
// If valid, it extracts the user ID and injects it into the request context.
func JWTMiddleware(tokenService service.TokenService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				http.Error(w, "invalid authorization header format", http.StatusUnauthorized)
				return
			}

			claims, err := tokenService.VerifyToken(parts[1])
			if err != nil {
				http.Error(w, "invalid or expired token", http.StatusUnauthorized)
				return
			}

			// Inject user public ID into the context
			ctx := context.WithValue(r.Context(), userIDKey, claims.UserPublicID.String())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext retrieves the user ID from the request context.
func UserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDKey).(string)
	return userID, ok
}
