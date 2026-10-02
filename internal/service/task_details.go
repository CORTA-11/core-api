package service

import "unicode/utf8"

// An omitted details field preserves existing text; an empty string clears it.
func taskDetailsValue(details *string) (string, error) {
	if details == nil {
		return "", nil
	}
	if !utf8.ValidString(*details) || utf8.RuneCountInString(*details) > 4096 {
		return "", ErrInvalidInput
	}
	return *details, nil
}
