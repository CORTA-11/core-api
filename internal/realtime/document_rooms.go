package realtime

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

const defaultCollaborationInternalURL = "http://127.0.0.1:8082"

// DocumentRoomCloser tells the collaboration service that a persisted Document was deleted.
type DocumentRoomCloser struct {
	baseURL       string
	client        *http.Client
	serviceSecret string
}

// NewDocumentRoomCloser constructs a private collaboration-service client.
func NewDocumentRoomCloser(baseURL, serviceSecret string, client *http.Client) (*DocumentRoomCloser, error) {
	if baseURL == "" {
		baseURL = defaultCollaborationInternalURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("COLLABORATION_INTERNAL_URL must be an HTTP origin or base URL")
	}
	if len([]byte(serviceSecret)) < 32 {
		return nil, fmt.Errorf("COLLABORATION_SERVICE_SECRET must contain at least 32 bytes")
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &DocumentRoomCloser{
		baseURL: strings.TrimSuffix(parsed.String(), "/"), client: client, serviceSecret: serviceSecret,
	}, nil
}

// CloseDocumentRoom terminates every Editing Session for one deleted Document.
func (closer *DocumentRoomCloser) CloseDocumentRoom(
	ctx context.Context, organizationID, teamID, documentID uuid.UUID,
) error {
	path := fmt.Sprintf("/internal/v1/orgs/%s/teams/%s/documents/%s/room", organizationID, teamID, documentID)
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, closer.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create collaboration room close request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+closer.serviceSecret)
	response, err := closer.client.Do(request)
	if err != nil {
		return fmt.Errorf("close collaboration room: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("collaboration room close failed with status %d", response.StatusCode)
	}
	return nil
}
