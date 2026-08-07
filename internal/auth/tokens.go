package auth

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strconv"
	"time"
)

const (
	defaultAccessTokenTTL  = 15 * time.Minute
	defaultRefreshTokenTTL = 7 * 24 * time.Hour
)

// AccessTokenTTL returns how long access JWTs remain valid.
func AccessTokenTTL() time.Duration {
	return durationFromEnv("ACCESS_TOKEN_TTL_MINUTES", defaultAccessTokenTTL, time.Minute)
}

// RefreshTokenTTL returns how long refresh sessions remain valid.
func RefreshTokenTTL() time.Duration {
	return durationFromEnv("REFRESH_TOKEN_TTL_HOURS", defaultRefreshTokenTTL, time.Hour)
}

// GenerateRefreshToken creates a high-entropy opaque refresh token.
func GenerateRefreshToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func durationFromEnv(key string, fallback time.Duration, unit time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}

	return time.Duration(value) * unit
}
