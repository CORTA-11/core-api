package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// In a real application, load this from your .env file!
var jwtSecret = []byte("your-super-secret-key-change-in-production")

// CustomClaims allows us to embed the user's ID and Role into the token payload
type CustomClaims struct {
	UserID  int64  `json:"user_id"`
	OrgID   int64  `json:"org_id"`
	OrgRole string `json:"org_role"`
	jwt.RegisteredClaims
}

// GenerateToken creates a signed JWT for the authenticated user
func GenerateToken(userID, orgID int64, orgRole string) (string, error) {
	claims := CustomClaims{
		UserID:  userID,
		OrgID:   orgID,
		OrgRole: orgRole,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func ValidateToken(tokenString string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Ensure the signing method is what we expect (HMAC)
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtSecret, nil
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
