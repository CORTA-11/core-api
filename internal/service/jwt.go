package service

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	// ErrInvalidToken is returned when a JWT token is expired, invalidly signed, or malformed.
	ErrInvalidToken = errors.New("invalid or expired token")
)

// JWTClaims represents the custom claims we store in the JWT.
type JWTClaims struct {
	UserPublicID uuid.UUID `json:"user_id"`
	Email        string    `json:"email"`
	jwt.RegisteredClaims
}

// TokenService manages the generation and verification of JWT authentication tokens.
type TokenService interface {
	GenerateToken(userPublicID uuid.UUID, email string) (string, error)
	VerifyToken(tokenString string) (*JWTClaims, error)
}
