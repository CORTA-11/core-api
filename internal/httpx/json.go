package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

var (
	ErrUnsupportedMediaType = errors.New("request content type must be application/json")
	ErrEmptyBody            = errors.New("request body must contain one JSON value")
	ErrBodyTooLarge         = errors.New("request body is too large")
	ErrMalformedJSON        = errors.New("request body contains malformed JSON")
	ErrUnknownField         = errors.New("request body contains an unknown field")
	ErrMultipleJSON         = errors.New("request body must contain a single JSON value")
	ErrBodyNotAllowed       = errors.New("request body is not allowed")
)

// DecodeJSON decodes json.
func DecodeJSON(request *http.Request, destination any, maxBytes int64) error {
	if err := request.Context().Err(); err != nil {
		return err
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		return ErrUnsupportedMediaType
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, maxBytes+1))
	if err != nil {
		if isMaxBytesError(err) {
			return ErrBodyTooLarge
		}
		return fmt.Errorf("read request body: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return ErrBodyTooLarge
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return ErrEmptyBody
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		if strings.HasPrefix(err.Error(), "json: unknown field ") {
			return ErrUnknownField
		}
		return ErrMalformedJSON
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return ErrMultipleJSON
		}
		return ErrMultipleJSON
	}
	if err := request.Context().Err(); err != nil {
		return err
	}
	return nil
}

// DecodeProblem decodes problem.
func DecodeProblem(err error) *AppError {
	violation := Violation{Field: "body", Code: "invalid", Message: "The request body is invalid."}
	switch {
	case errors.Is(err, ErrUnsupportedMediaType):
		violation = Violation{"body", "media_type", "The request Content-Type must be application/json."}
	case errors.Is(err, ErrEmptyBody):
		violation = Violation{"body", "required", "The request body must contain one JSON object."}
	case errors.Is(err, ErrBodyTooLarge):
		violation = Violation{"body", "too_large", "The request body exceeds the allowed size."}
	case errors.Is(err, ErrBodyNotAllowed):
		violation = Violation{"body", "not_allowed", "A request body is not allowed for this operation."}
	case errors.Is(err, ErrUnknownField):
		violation = Violation{"body", "unknown_field", "The request body contains an unknown field."}
	case errors.Is(err, ErrMalformedJSON), errors.Is(err, ErrMultipleJSON):
		violation = Violation{"body", "malformed", "The request body must contain one valid JSON object."}
	}
	return NewError(ProblemInvalidRequest, err, violation)
}

// WriteJSON writes json.
func WriteJSON(writer http.ResponseWriter, status int, value any) error {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(true)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode JSON response: %w", err)
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if _, err := writer.Write(buffer.Bytes()); err != nil {
		return fmt.Errorf("write JSON response: %w", err)
	}
	return nil
}
