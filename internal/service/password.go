package service

// PasswordService defines methods for secure password hashing and verification.
type PasswordService interface {
	HashPassword(password string) (string, error)
	VerifyPassword(password, encodedHash string) (bool, error)
}
