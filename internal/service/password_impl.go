package service

import (
	"context"

	"github.com/CORTA-11/core-api/internal/identity"
)

type passwordService struct {
	hasher identity.PasswordHasher
	policy identity.PasswordPolicy
}

// NewPasswordService preserves the prototype's context-free helper surface
// while delegating all cryptographic work to the bounded identity hasher.
func NewPasswordService() PasswordService {
	hasher, err := identity.NewPasswordHasher(identity.HashConfig{})
	if err != nil {
		panic("default password hash configuration is invalid")
	}
	return &passwordService{hasher: hasher}
}

func (service *passwordService) HashPassword(password string) (string, error) {
	normalized, err := service.policy.Normalize(password)
	if err != nil {
		return "", err
	}
	return service.hasher.Hash(context.Background(), normalized)
}

func (service *passwordService) VerifyPassword(password, encodedHash string) (bool, error) {
	verification, err := service.hasher.Verify(context.Background(), password, encodedHash)
	return verification.Match, err
}
