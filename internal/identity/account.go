package identity

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const maximumDisplayNameCodePoints = 100

var ErrInvalidDisplayName = errors.New("invalid display name")

func NormalizeDisplayName(value string) (string, error) {
	if !utf8.ValidString(value) || len(value) > 255 {
		return "", ErrInvalidDisplayName
	}
	normalized := norm.NFC.String(strings.TrimSpace(value))
	codePoints := utf8.RuneCountInString(normalized)
	if codePoints == 0 || codePoints > maximumDisplayNameCodePoints || len(normalized) > 255 {
		return "", ErrInvalidDisplayName
	}
	for _, character := range normalized {
		if unicode.IsControl(character) {
			return "", ErrInvalidDisplayName
		}
	}
	return normalized, nil
}
