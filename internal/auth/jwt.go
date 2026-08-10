package auth

import (
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func jwtSecret() []byte {
	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		return []byte(secret)
	}
	// Dev default — keep in sync with socket-server.
	return []byte("your-super-secret-key-change-in-production")
}

// CustomClaims allows us to embed the user's ID and Role into the token payload
type CustomClaims struct {
	UserID  int64  `json:"user_id"`
	OrgID   int64  `json:"org_id"`
	OrgRole string `json:"org_role"`
	jwt.RegisteredClaims
}

// GenerateToken creates a signed JWT for the authenticated user
func GenerateToken(userID, orgID int64, orgRole string) (string, error) {
	now := time.Now()
	claims := CustomClaims{
		UserID:  userID,
		OrgID:   orgID,
		OrgRole: orgRole,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL())),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret())
}

func ValidateToken(tokenString string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Ensure the signing method is what we expect (HMAC)
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtSecret(), nil
	})

	if err != nil {
		return nil, err
	}

	// Extract and return the custom claims if the token is valid
	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}
