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
	request.Header.Set("Content-Type", "application/json")
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
		{"empty", ``, 100, ErrEmptyBody},
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
	request.Header.Set("Content-Type", "application/json")
	assert.ErrorIs(t, DecodeJSON(request, &payload{}, 100), context.Canceled)
}

func TestDecodeJSONRequiresApplicationJSON(t *testing.T) {
	for _, contentType := range []string{"", "text/plain", "application/problem+json", "not a media type"} {
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"value"}`))
		request.Header.Set("Content-Type", contentType)
		assert.ErrorIs(t, DecodeJSON(request, &payload{}, 100), ErrUnsupportedMediaType)
	}
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"value"}`))
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	assert.NoError(t, DecodeJSON(request, &payload{}, 100))
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
	err := NewError(ProblemInvalidRequest, errors.New("submitted secret value"),
		Violation{Field: "name", Code: "required", Message: "The name is required."})
	require.NoError(t, WriteProblem(recorder, httptest.NewRequest(http.MethodGet, "/", nil), err))
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "The name is required.")
	assert.NotContains(t, recorder.Body.String(), "submitted secret")
	assert.Contains(t, recorder.Body.String(), `"type":"/problems/invalid-request"`)
}

func TestWriteProblemOmitsInvalidAndBoundsValidViolations(t *testing.T) {
	violations := []Violation{
		{Field: strings.Repeat("f", 65), Code: "bad", Message: "invalid field"},
		{Field: "name", Code: "bad", Message: strings.Repeat("m", 257)},
	}
	for range 20 {
		violations = append(violations, Violation{Field: "body", Code: "invalid", Message: "Invalid input."})
	}
	problem := ProblemFromError(httptest.NewRequest(http.MethodGet, "/", nil),
		NewError(ProblemInvalidRequest, nil, violations...))
	assert.Len(t, problem.Violations, 16)
}

func TestRecoverDiscardsPartialWrites(t *testing.T) {
	handler := RequestID(Recover(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte("secret partial response"))
		panic("database secret")
	})))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Equal(t, "application/problem+json", recorder.Header().Get("Content-Type"))
	assert.NotContains(t, recorder.Body.String(), "secret")
}
