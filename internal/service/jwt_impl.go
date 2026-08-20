package service

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type tokenService struct {
	secret []byte
}

// NewTokenService creates a new instance of TokenService.
func NewTokenService(secret string) TokenService {
	return &tokenService{
		secret: []byte(secret),
	}
}

// GenerateToken generates an HMAC-SHA256 JWT token for the user, expiring in 24 hours.
func (s *tokenService) GenerateToken(userPublicID uuid.UUID, email string) (string, error) {
	claims := JWTClaims{
		UserPublicID: userPublicID,
		Email:        email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

// VerifyToken validates and decodes the claims of the JWT token string.
func (s *tokenService) VerifyToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return s.secret, nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}
