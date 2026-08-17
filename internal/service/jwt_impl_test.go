package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenService(t *testing.T) {
	s := NewTokenService()
	userPublicID := uuid.New()
	email := "user@example.com"

	t.Run("generates a token successfully", func(t *testing.T) {
		token, err := s.GenerateToken(userPublicID, email)
		require.NoError(t, err)
		assert.NotEmpty(t, token)
	})

	t.Run("verifies token successfully", func(t *testing.T) {
		token, err := s.GenerateToken(userPublicID, email)
		require.NoError(t, err)

		claims, err := s.VerifyToken(token)
		require.NoError(t, err)
		assert.Equal(t, userPublicID, claims.UserPublicID)
		assert.Equal(t, email, claims.Email)
	})

	t.Run("fails verification for invalid signature", func(t *testing.T) {
		token, err := s.GenerateToken(userPublicID, email)
		require.NoError(t, err)

		corruptedToken := token + "invalid"
		_, err = s.VerifyToken(corruptedToken)
		assert.Error(t, err)
	})

	t.Run("fails verification for invalid token format", func(t *testing.T) {
		_, err := s.VerifyToken("totally.invalid.token")
		assert.Error(t, err)
	})
}
