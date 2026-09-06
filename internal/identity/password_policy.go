package identity

import (
	"errors"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	minimumPasswordCodePoints = 12
	maximumPasswordCodePoints = 128
	maximumPasswordBytes      = 1024
)

// ErrPasswordPolicy is intentionally stable and never carries password input.
var ErrPasswordPolicy = errors.New("password does not satisfy policy")

// PasswordPolicy validates password input and returns its versioned storage
// form. The policy deliberately permits spaces and all non-control Unicode.
type PasswordPolicy struct{}

// Normalize validates UTF-8 and resource bounds, normalizes the password to
// NFC, and enforces the code-point and control-character policy.
func (PasswordPolicy) Normalize(password string) (string, error) {
	if !utf8.ValidString(password) || len(password) > maximumPasswordBytes {
		return "", ErrPasswordPolicy
	}

	normalized := norm.NFC.String(password)
	if len(normalized) > maximumPasswordBytes {
		return "", ErrPasswordPolicy
	}
	codePoints := utf8.RuneCountInString(normalized)
	if codePoints < minimumPasswordCodePoints || codePoints > maximumPasswordCodePoints {
		return "", ErrPasswordPolicy
	}
	for _, character := range normalized {
		if unicode.IsControl(character) {
			return "", ErrPasswordPolicy
		}
	}
	return normalized, nil
}
