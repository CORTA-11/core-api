package middleware

import (
	"net/http"
	"strings"

	"github.com/CORTA-11/core-api/internal/auth"
)

// RequireAuth intercepts requests, validates the JWT, and sets the user context.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		// Expecting "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "invalid authorization header format", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]
		claims, err := auth.ValidateToken(tokenString)
		if err != nil {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Inject the claims into the request context
		ctx := auth.ContextWithClaims(r.Context(), claims)

		// Pass the request to the next handler with the new context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRoles checks if the authenticated user has one of the required roles.
// This middleware MUST be placed after RequireAuth in the middleware chain.
func RequireRoles(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Retrieve claims from context (injected by RequireAuth)
			claims, ok := auth.ClaimsFromContext(r.Context())
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			// Check if the user's role matches any of the allowed roles
			hasRole := false
			for _, role := range allowedRoles {
				if claims.OrgRole == role {
					hasRole = true
					break
				}
			}

			// If the role doesn't match, block the request
			if !hasRole {
				http.Error(w, "forbidden: insufficient permissions", http.StatusForbidden)
				return
			}

			// User is authorized, proceed to the handler
			next.ServeHTTP(w, r)
		})
	}
}
