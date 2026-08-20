package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var (
	ErrBodyTooLarge  = errors.New("request body is too large")
	ErrMalformedJSON = errors.New("request body contains malformed JSON")
	ErrUnknownField  = errors.New("request body contains an unknown field")
	ErrMultipleJSON  = errors.New("request body must contain a single JSON value")
)

func DecodeJSON(request *http.Request, destination any, maxBytes int64) error {
	if err := request.Context().Err(); err != nil {
		return err
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, maxBytes+1))
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return ErrBodyTooLarge
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		if strings.HasPrefix(err.Error(), "json: unknown field ") {
			return fmt.Errorf("%w: %s", ErrUnknownField, strings.TrimPrefix(err.Error(), "json: unknown field "))
		}
		return fmt.Errorf("%w: %v", ErrMalformedJSON, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return ErrMultipleJSON
		}
		return fmt.Errorf("%w: %v", ErrMultipleJSON, err)
	}
	if err := request.Context().Err(); err != nil {
		return err
	}
	return nil
}

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
