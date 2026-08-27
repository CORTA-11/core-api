package httpx

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/CORTA-11/core-api/internal/apicontract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServerAppliesBoundedSettings(t *testing.T) {
	server := NewServer(":8080", http.NotFoundHandler(), ServerTimeouts{
		ReadHeader: time.Second, Read: 2 * time.Second, Write: 3 * time.Second, Idle: 4 * time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	assert.Equal(t, time.Second, server.ReadHeaderTimeout)
	assert.Equal(t, 2*time.Second, server.ReadTimeout)
	assert.Equal(t, 3*time.Second, server.WriteTimeout)
	assert.Equal(t, 4*time.Second, server.IdleTimeout)
	assert.Equal(t, 32<<10, server.MaxHeaderBytes)
	require.NotNil(t, server.ErrorLog)
}

func TestBodyLimitUsesReviewedRouteClass(t *testing.T) {
	tests := []struct {
		name   string
		class  apicontract.BodyLimitClass
		size   int
		status int
	}{
		{"no body rejects body", apicontract.BodyNone, 1, http.StatusBadRequest},
		{"auth allows 4 KiB", apicontract.BodyAuthJSON, 4 << 10, http.StatusNoContent},
		{"auth rejects excess", apicontract.BodyAuthJSON, (4 << 10) + 1, http.StatusBadRequest},
		{"resource allows 64 KiB", apicontract.BodyJSON, 64 << 10, http.StatusNoContent},
		{"resource rejects excess", apicontract.BodyJSON, (64 << 10) + 1, http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := LimitBody(test.class, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				_, err := io.ReadAll(request.Body)
				if err != nil {
					_ = WriteProblem(writer, request, DecodeProblem(err))
					return
				}
				writer.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bytes.Repeat([]byte("x"), test.size)))
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			assert.Equal(t, test.status, recorder.Code)
		})
	}
}

func TestServerRejectsOversizedAndPartialHeadersAndRemainsHealthy(t *testing.T) {
	server := NewServer("", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.ReadAll(request.Body)
		writer.WriteHeader(http.StatusNoContent)
	}), ServerTimeouts{ReadHeader: 40 * time.Millisecond, Read: 80 * time.Millisecond, Write: time.Second, Idle: time.Second}, slog.Default())
	socketPath := filepath.Join(t.TempDir(), "http.sock")
	listener, err := net.Listen("unix", socketPath)
	if errors.Is(err, syscall.EPERM) {
		t.Skip("sandbox forbids sockets")
	}
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Shutdown(context.Background())
		<-done
	})

	connection, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	_, err = io.WriteString(connection, "GET / HTTP/1.1\r\nHost: test\r\nX-Large: "+strings.Repeat("a", 40<<10)+"\r\n\r\n")
	require.NoError(t, err)
	response, err := http.ReadResponse(bufio.NewReader(connection), nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusRequestHeaderFieldsTooLarge, response.StatusCode)
	_ = response.Body.Close()
	_ = connection.Close()

	partial, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	_, err = io.WriteString(partial, "GET / HTTP/1.1\r\nHost: test")
	require.NoError(t, err)
	require.NoError(t, partial.SetReadDeadline(time.Now().Add(time.Second)))
	_, err = io.ReadAll(partial)
	require.NoError(t, err)
	_ = partial.Close()

	partialBody, err := net.Dial("unix", socketPath)
	require.NoError(t, err)
	_, err = io.WriteString(partialBody, "POST / HTTP/1.1\r\nHost: test\r\nContent-Length: 10\r\n\r\nx")
	require.NoError(t, err)
	require.NoError(t, partialBody.SetReadDeadline(time.Now().Add(time.Second)))
	_, err = io.ReadAll(partialBody)
	require.NoError(t, err)
	_ = partialBody.Close()

	client := &http.Client{Transport: &http.Transport{DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
		return net.Dial("unix", socketPath)
	}}}
	response, err = client.Get("http://unix/")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, response.StatusCode)
	_ = response.Body.Close()
}
