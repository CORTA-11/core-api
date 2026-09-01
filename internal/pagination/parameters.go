// Package pagination implements the bounded v1 page parameters and signed,
// scope-bound keyset cursors.
package pagination

import (
	"errors"
	"net/url"
	"strconv"
)

const (
	DefaultPageSize  = 50
	MaximumPageSize  = 100
	MaximumTokenSize = 512
)

var ErrInvalidParameters = errors.New("invalid pagination parameters")

type Parameters struct {
	PageSize int
	Cursor   string
}

// Parse parses the supplied values.
func Parse(values url.Values) (Parameters, error) {
	result := Parameters{PageSize: DefaultPageSize}
	if _, exists := values["offset"]; exists {
		return Parameters{}, ErrInvalidParameters
	}
	if pageSizes, exists := values["page_size"]; exists {
		if len(pageSizes) != 1 || pageSizes[0] == "" {
			return Parameters{}, ErrInvalidParameters
		}
		pageSize, err := strconv.Atoi(pageSizes[0])
		if err != nil || pageSize < 1 || pageSize > MaximumPageSize {
			return Parameters{}, ErrInvalidParameters
		}
		result.PageSize = pageSize
	}
	if cursors, exists := values["cursor"]; exists {
		if len(cursors) != 1 || cursors[0] == "" || len(cursors[0]) > MaximumTokenSize {
			return Parameters{}, ErrInvalidParameters
		}
		result.Cursor = cursors[0]
	}
	return result, nil
}
