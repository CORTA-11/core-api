package service

// PasswordService defines methods for secure password hashing and verification.
type PasswordService interface {
	HashPassword(password string) (string, error)
	VerifyPassword(password, encodedHash string) (bool, error)
}

// HashPassword hashes a password using the default password service settings.
// It is kept as a package-level convenience API for callers that do not need
// to inject a PasswordService.
func HashPassword(password string) (string, error) {
	return NewPasswordService().HashPassword(password)
}

// VerifyPassword verifies a password using the parameters encoded in the hash.
func VerifyPassword(password, encodedHash string) (bool, error) {
	return NewPasswordService().VerifyPassword(password, encodedHash)
}
