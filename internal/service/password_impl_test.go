package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPasswordService(t *testing.T) {
	s := NewPasswordService()
	password := "my-secure-password"

	t.Run("hashes password successfully", func(t *testing.T) {
		hash, err := s.HashPassword(password)
		require.NoError(t, err)
		assert.NotEmpty(t, hash)
		assert.Contains(t, hash, "$argon2id$")
	})

	t.Run("verifies correct password", func(t *testing.T) {
		hash, err := s.HashPassword(password)
		require.NoError(t, err)

		match, err := s.VerifyPassword(password, hash)
		require.NoError(t, err)
		assert.True(t, match)
	})

	t.Run("rejects incorrect password", func(t *testing.T) {
		hash, err := s.HashPassword(password)
		require.NoError(t, err)

		match, err := s.VerifyPassword("wrong-password", hash)
		require.NoError(t, err)
		assert.False(t, match)
	})

	t.Run("fails on invalid hash format", func(t *testing.T) {
		match, err := s.VerifyPassword(password, "invalid-hash-string")
		assert.Error(t, err)
		assert.False(t, match)
	})
}
