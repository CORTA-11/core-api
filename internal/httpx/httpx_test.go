package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type payload struct {
	Name string `json:"name"`
}

func decodeRequest(body string, limit int64) error {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	return DecodeJSON(request, &payload{}, limit)
}

func TestDecodeJSONRejectsInvalidBodies(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		limit int64
		want  error
	}{
		{"malformed", `{"name":`, 100, ErrMalformedJSON},
		{"unknown field", `{"other":"value"}`, 100, ErrUnknownField},
		{"multiple values", `{"name":"one"} {"name":"two"}`, 100, ErrMultipleJSON},
		{"oversized", `{"name":"too large"}`, 5, ErrBodyTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) { assert.ErrorIs(t, decodeRequest(test.body, test.limit), test.want) })
	}
}

func TestDecodeJSONHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"value"}`)).WithContext(ctx)
	assert.ErrorIs(t, DecodeJSON(request, &payload{}, 100), context.Canceled)
}

func TestWriteJSONBuffersEncoderFailure(t *testing.T) {
	recorder := httptest.NewRecorder()
	err := WriteJSON(recorder, http.StatusCreated, make(chan int))
	require.Error(t, err)
	assert.Empty(t, recorder.Body.String())
	assert.Empty(t, recorder.Header().Get("Content-Type"))
}

func TestRequestIDIsGeneratedAndPropagated(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(RequestIDFromContext(request.Context())))
	}))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(RequestIDHeader, "untrusted-client-value")
	handler.ServeHTTP(recorder, request)
	assert.NotEmpty(t, recorder.Header().Get(RequestIDHeader))
	assert.NotEqual(t, "untrusted-client-value", recorder.Header().Get(RequestIDHeader))
	assert.Equal(t, recorder.Header().Get(RequestIDHeader), recorder.Body.String())
}

func TestWriteProblemRedactsInternalErrors(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, WriteProblem(recorder, request, errors.New("database password is secret")))
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "database password")
	assert.Contains(t, recorder.Header().Get("Content-Type"), "application/problem+json")
}

func TestWriteProblemExposesTypedSafeDetail(t *testing.T) {
	recorder := httptest.NewRecorder()
	err := &AppError{Status: http.StatusBadRequest, Type: "https://example.com/problems/invalid", Title: "Invalid request", Detail: "The name is required."}
	require.NoError(t, WriteProblem(recorder, httptest.NewRequest(http.MethodGet, "/", nil), err))
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "The name is required.")
}
