package realtime

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentRoomCloserAuthenticatesAndScopesDeletion(t *testing.T) {
	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	teamID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	documentID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	var request *http.Request
	client := &http.Client{Transport: roundTripFunc(func(received *http.Request) (*http.Response, error) {
		request = received
		return response(http.StatusNoContent, ""), nil
	})}
	closer, err := NewDocumentRoomCloser("http://collaboration.internal:8082", "test-collaboration-service-secret-123", client)
	require.NoError(t, err)

	require.NoError(t, closer.CloseDocumentRoom(context.Background(), organizationID, teamID, documentID))

	assert.Equal(t, http.MethodDelete, request.Method)
	assert.Equal(t, "/internal/v1/orgs/"+organizationID.String()+"/teams/"+teamID.String()+"/documents/"+documentID.String()+"/room", request.URL.Path)
	assert.Equal(t, "Bearer test-collaboration-service-secret-123", request.Header.Get("Authorization"))
}

func TestDocumentRoomCloserSurfacesFailureWithoutResponseContent(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusInternalServerError, "ticket-and-document-content-must-not-leak"), nil
	})}
	closer, err := NewDocumentRoomCloser("http://collaboration.internal:8082", "test-collaboration-service-secret-123", client)
	require.NoError(t, err)

	err = closer.CloseDocumentRoom(context.Background(), uuid.New(), uuid.New(), uuid.New())

	assert.EqualError(t, err, "collaboration room close failed with status 500")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func response(status int, body string) *http.Response {
	return &http.Response{
		Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), StatusCode: status,
	}
}
