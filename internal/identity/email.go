// Package identity owns canonical email and local credential behavior.
package identity

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const maxEmailBytes = 254

// ErrInvalidEmail reports an email value that cannot be represented as a
// canonical identity. It deliberately does not include the rejected value.
var ErrInvalidEmail = errors.New("invalid email")

// CanonicalEmail keeps the normalized display spelling separate from the key
// used for uniqueness and lookup.
type CanonicalEmail struct {
	Display string
	Key     string
}

// EmailCanonicalizer applies the same normalization pipeline as PostgreSQL's
// public.canonical_email_key function.
type EmailCanonicalizer struct{}

// Canonicalize trims surrounding ASCII spaces, normalizes the display form to
// NFC, and applies Unicode default case folding followed by NFC to form the key.
func (EmailCanonicalizer) Canonicalize(value string) (CanonicalEmail, error) {
	if !utf8.ValidString(value) || len(value) > 4*maxEmailBytes {
		return CanonicalEmail{}, ErrInvalidEmail
	}

	display := norm.NFC.String(strings.Trim(value, " "))
	if display == "" || len(display) > maxEmailBytes {
		return CanonicalEmail{}, ErrInvalidEmail
	}
	for _, character := range display {
		if unicode.IsControl(character) {
			return CanonicalEmail{}, ErrInvalidEmail
		}
	}

	key := norm.NFC.String(cases.Fold().String(display))
	if key == "" || len(key) > maxEmailBytes {
		return CanonicalEmail{}, ErrInvalidEmail
	}
	return CanonicalEmail{Display: display, Key: key}, nil
}
