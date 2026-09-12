package httpx

import (
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	maximumViolations      = 16
	maximumViolationName   = 64
	maximumViolationDetail = 256
)

type ProblemKind string

const (
	ProblemInvalidRequest        ProblemKind = "invalid-request"
	ProblemUnauthenticated       ProblemKind = "unauthenticated"
	ProblemForbidden             ProblemKind = "forbidden"
	ProblemNotFound              ProblemKind = "not-found"
	ProblemConflict              ProblemKind = "conflict"
	ProblemPreconditionFailed    ProblemKind = "precondition-failed"
	ProblemRateLimited           ProblemKind = "rate-limited"
	ProblemInternalFailure       ProblemKind = "internal-failure"
	ProblemDependencyUnavailable ProblemKind = "dependency-unavailable"
)

type problemDefinition struct {
	Status int
	Title  string
	Detail string
}

var problemRegistry = map[ProblemKind]problemDefinition{
	ProblemInvalidRequest:        {http.StatusBadRequest, "Invalid request", "The request is invalid."},
	ProblemUnauthenticated:       {http.StatusUnauthorized, "Authentication required", "Authentication is required."},
	ProblemForbidden:             {http.StatusForbidden, "Forbidden", "The operation is not permitted."},
	ProblemNotFound:              {http.StatusNotFound, "Not found", "The requested resource was not found."},
	ProblemConflict:              {http.StatusConflict, "Conflict", "The request conflicts with current state."},
	ProblemPreconditionFailed:    {http.StatusPreconditionFailed, "Precondition failed", "A request precondition was not met."},
	ProblemRateLimited:           {http.StatusTooManyRequests, "Too many requests", "Too many requests were received."},
	ProblemInternalFailure:       {http.StatusInternalServerError, "Internal server error", "An unexpected error occurred."},
	ProblemDependencyUnavailable: {http.StatusServiceUnavailable, "Dependency unavailable", "A required dependency is unavailable."},
}

type Violation struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type AppError struct {
	kind       ProblemKind
	violations []Violation
	cause      error
}

func NewError(kind ProblemKind, cause error, violations ...Violation) *AppError {
	if _, ok := problemRegistry[kind]; !ok {
		kind = ProblemInternalFailure
	}
	return &AppError{kind: kind, violations: append([]Violation(nil), violations...), cause: cause}
}

func (err *AppError) Error() string {
	if err.cause != nil {
		return err.cause.Error()
	}
	return string(err.kind)
}

func (err *AppError) Unwrap() error { return err.cause }

type Problem struct {
	Type       string      `json:"type"`
	Title      string      `json:"title"`
	Status     int         `json:"status"`
	Detail     string      `json:"detail"`
	RequestID  string      `json:"request_id"`
	Violations []Violation `json:"violations,omitempty"`
}

func ProblemFromError(request *http.Request, err error) Problem {
	kind := ProblemInternalFailure
	var appError *AppError
	if errors.As(err, &appError) {
		kind = appError.kind
	}
	definition, ok := problemRegistry[kind]
	if !ok {
		definition = problemRegistry[ProblemInternalFailure]
		kind = ProblemInternalFailure
	}
	requestID := RequestIDFromContext(request.Context())
	if requestID == "" {
		requestID = uuid.NewString()
	}
	problem := Problem{
		Type: "/problems/" + string(kind), Title: definition.Title,
		Status: definition.Status, Detail: definition.Detail, RequestID: requestID,
	}
	if appError != nil {
		problem.Violations = safeViolations(appError.violations)
	}
	return problem
}

func WriteProblem(writer http.ResponseWriter, request *http.Request, err error) error {
	problem := ProblemFromError(request, err)
	var buffer responseBuffer
	if err := WriteJSON(&buffer, problem.Status, problem); err != nil {
		return err
	}
	writer.Header().Set("Content-Type", "application/problem+json")
	writer.Header().Set("Cache-Control", "no-store")
	if writer.Header().Get(RequestIDHeader) == "" {
		writer.Header().Set(RequestIDHeader, problem.RequestID)
	}
	writer.WriteHeader(problem.Status)
	_, writeErr := writer.Write(buffer.body.Bytes())
	return writeErr
}

func safeViolations(input []Violation) []Violation {
	result := make([]Violation, 0, min(len(input), maximumViolations))
	for _, violation := range input {
		if len(result) == maximumViolations {
			break
		}
		if validBoundedText(violation.Field, maximumViolationName) &&
			validBoundedText(violation.Code, maximumViolationName) &&
			validBoundedText(violation.Message, maximumViolationDetail) {
			result = append(result, violation)
		}
	}
	return result
}

func validBoundedText(value string, maximum int) bool {
	return value != "" && utf8.ValidString(value) && len([]byte(value)) <= maximum
}
