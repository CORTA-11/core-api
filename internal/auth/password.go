package auth

import "github.com/alexedwards/argon2id"

// CreateHash generates an Argon2id hash from a plain-text password.
func CreateHash(password string) (string, error) {
	// Default parameters are safe and recommended by OWASP
	hash, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return "", err
	}
	return hash, nil
}

// ComparePasswordAndHash checks if the plain-text password matches the given hash.
func ComparePasswordAndHash(password, hash string) (bool, error) {
	match, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return false, err
	}
	return match, nil
}
