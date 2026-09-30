package push

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testRoundTripper func(*http.Request) (*http.Response, error)

func (transport testRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type contextPruner struct{ err error }

func (pruner *contextPruner) DeleteDeviceToken(ctx context.Context, _ string) error {
	pruner.err = ctx.Err()
	return nil
}

func TestMulticastPruningRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pruner := &contextPruner{}
	service := &FCMService{
		projectID: "test", pruner: pruner, logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		client: &http.Client{Transport: testRoundTripper(func(request *http.Request) (*http.Response, error) {
			cancel()
			return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"error":{"status":"UNREGISTERED"}}`)), Request: request}, nil
		})},
	}
	require.NoError(t, service.SendMulticast(ctx, []string{"test-token"}, NotificationPayload{}))
	require.ErrorIs(t, pruner.err, context.Canceled)
}

func TestCredentialsFileCannotEscapeDirectory(t *testing.T) {
	directory := t.TempDir()
	outside := filepath.Join(t.TempDir(), "credentials.json")
	require.NoError(t, os.WriteFile(outside, []byte("{}"), 0600))
	path := filepath.Join(directory, "credentials.json")
	require.NoError(t, os.Symlink(outside, path))
	_, err := NewFCMServiceFromFile(context.Background(), path, nil, nil)
	require.ErrorContains(t, err, "read firebase credentials file")
}

func TestCredentialsFileReadsWithinDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	require.NoError(t, os.WriteFile(path, []byte("invalid json"), 0600))
	_, err := NewFCMServiceFromFile(context.Background(), path, nil, nil)
	require.ErrorContains(t, err, "parse firebase credentials")
}

func TestNoopService(t *testing.T) {
	svc := &NoopService{}
	err := svc.SendMulticast(context.Background(), []string{"test-token"}, NotificationPayload{
		Title: "Test",
		Body:  "Body",
	})
	assert.NoError(t, err)
}

func TestNewFCMServiceFromFile(t *testing.T) {
	secretPath := "../../dev_secrets/firebase_service_account.json"
	if _, err := os.Stat(secretPath); os.IsNotExist(err) {
		t.Skip("skipping integration test; dev_secrets/firebase_service_account.json not found")
	}

	svc, err := NewFCMServiceFromFile(context.Background(), secretPath, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "corta-50e32", svc.projectID)
	assert.NotNil(t, svc.client)
}
