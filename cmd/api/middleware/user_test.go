package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CORTA-11/core-api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWTMiddleware(t *testing.T) {
	userPublicID := uuid.New()
	email := "test@example.com"
	tokenService := service.NewTokenService("unit-test-jwt-secret")

	// Generate a valid token for testing
	validToken, err := tokenService.GenerateToken(userPublicID, email)
	require.NoError(t, err)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := UserIDFromContext(r.Context())
		assert.True(t, ok)
		assert.Equal(t, userPublicID.String(), userID)
		w.WriteHeader(http.StatusOK)
	})

	handler := JWTMiddleware(tokenService)(nextHandler)

	t.Run("allows requests with a valid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("rejects requests with missing authorization header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Contains(t, rec.Body.String(), "missing authorization header")
	})

	t.Run("rejects requests with invalid authorization format", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		req.Header.Set("Authorization", "Basic "+validToken)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid authorization header format")
	})

	t.Run("rejects requests with an invalid/expired token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		req.Header.Set("Authorization", "Bearer invalid-token-string")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid or expired token")
	})
}
