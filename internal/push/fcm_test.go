package push

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
