package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecoveryFlushCommitsAndSubsequentWritesStream(t *testing.T) {
	recorder := httptest.NewRecorder()
	handler := Recover(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		_, err := writer.Write([]byte("data: first\n\n"))
		require.NoError(t, err)
		assert.Empty(t, recorder.Body.String())
		require.NoError(t, http.NewResponseController(writer).Flush())
		assert.True(t, recorder.Flushed)
		assert.Equal(t, "data: first\n\n", recorder.Body.String())
		_, err = writer.Write([]byte("data: second\n\n"))
		require.NoError(t, err)
		assert.Contains(t, recorder.Body.String(), "data: second")
	}))
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, "data: first\n\ndata: second\n\n", recorder.Body.String())
}

func TestRecoveryAbortsPanicAfterFlushWithoutAppendingProblem(t *testing.T) {
	recorder := httptest.NewRecorder()
	handler := Recover(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("data: first\n\n"))
		require.NoError(t, http.NewResponseController(writer).Flush())
		panic("internal error")
	}))
	assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	})
	assert.Equal(t, "data: first\n\n", recorder.Body.String())
}
