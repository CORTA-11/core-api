package querycheck

import (
	"fmt"
	"regexp"
)

var (
	queryHeaderPattern    = regexp.MustCompile(`(?m)^--\s*name:\s*([A-Za-z][A-Za-z0-9_]*)\s+:(one|many|exec|execrows|execlastid|copyfrom)\s*$`)
	selectWildcardPattern = regexp.MustCompile(`(?is)(?:\bselect\s+(?:distinct\s+)?|,\s*)(?:[a-z_][a-z0-9_$]*\.)?\*\s*(?:,|\bfrom\b)`)
	returnWildcardPattern = regexp.MustCompile(`(?is)\breturning\s+(?:[a-z_][a-z0-9_$]*\.)?\*`)
	orderByPattern        = regexp.MustCompile(`(?is)\border\s+by\b`)
	parameterLimitPattern = regexp.MustCompile(`(?is)\blimit\s+(?:sqlc\.(?:arg|narg)\s*\(\s*['"]?[a-z_][a-z0-9_]*['"]?\s*\)|\$\d+|[:@][a-z_][a-z0-9_]*|\?)`)
)

// Issue identifies a query-source invariant violation.
type Issue struct {
	Path    string
	Query   string
	Message string
}

func (i Issue) Error() string {
	return fmt.Sprintf("%s: query %s: %s", i.Path, i.Query, i.Message)
}

// Check validates all sqlc query blocks in source.
func Check(path string, source []byte) []Issue {
	headers := queryHeaderPattern.FindAllSubmatchIndex(source, -1)
	issues := make([]Issue, 0)

	for index, header := range headers {
		blockEnd := len(source)
		if index+1 < len(headers) {
			blockEnd = headers[index+1][0]
		}

		name := string(source[header[2]:header[3]])
		command := string(source[header[4]:header[5]])
		query := source[header[1]:blockEnd]

		if selectWildcardPattern.Match(query) {
			issues = append(issues, Issue{Path: path, Query: name, Message: "wildcard projection is not allowed"})
		}
		if returnWildcardPattern.Match(query) {
			issues = append(issues, Issue{Path: path, Query: name, Message: "wildcard RETURNING is not allowed"})
		}
		if command != "many" {
			continue
		}
		if !orderByPattern.Match(query) {
			issues = append(issues, Issue{Path: path, Query: name, Message: "missing ORDER BY"})
		}
		if !parameterLimitPattern.Match(query) {
			issues = append(issues, Issue{Path: path, Query: name, Message: "missing parameterized LIMIT"})
		}
	}

	return issues
}
