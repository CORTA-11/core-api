package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
)

const socketTicketTTL = 5 * time.Minute

var socketTicketHeader = map[string]string{"alg": "HS256", "typ": "JWT"}

type SocketTicketClaims struct {
	UserID       uuid.UUID `json:"user_id"`
	OrgID        uuid.UUID `json:"org_id"`
	TeamID       int64     `json:"team_id"`
	TeamPublicID uuid.UUID `json:"team_public_id"`
	ExpiresAt    int64     `json:"exp"`
}

func (application *ChatApplication) IssueSocketTicket(ctx context.Context, principal session.Principal, orgID, teamID uuid.UUID) (string, error) {
	if len(application.ticketSecret) < 32 {
		return "", errors.New("socket ticket secret is not configured")
	}
	var internalTeamID int64
	err := application.inTeam(ctx, principal, orgID, teamID, func(queries *tenantdb.Queries, _ rosterMap, _ string) error {
		var err error
		internalTeamID, err = queries.GetBoundTeamID(ctx)
		return err
	})
	if err != nil {
		return "", err
	}
	return signSocketTicket(SocketTicketClaims{
		UserID: principal.UserID, OrgID: orgID, TeamID: internalTeamID,
		TeamPublicID: teamID, ExpiresAt: time.Now().Add(socketTicketTTL).Unix(),
	}, application.ticketSecret)
}

func signSocketTicket(claims SocketTicketClaims, secret []byte) (string, error) {
	header, err := encodeSocketTicketPart(socketTicketHeader)
	if err != nil {
		return "", err
	}
	payload, err := encodeSocketTicketPart(claims)
	if err != nil {
		return "", err
	}
	unsigned := header + "." + payload
	signature := hmac.New(sha256.New, secret)
	_, _ = signature.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature.Sum(nil)), nil
}

func encodeSocketTicketPart(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func socketTicketSecret(secret string) []byte {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		secret = "development-socket-ticket-secret-change-me"
	}
	return []byte(secret)
}

func socketTicketForbidden(err error) error {
	if errors.Is(err, authorization.ErrUnauthenticated) || errors.Is(err, authorization.ErrOperationDenied) ||
		errors.Is(err, authorization.ErrResourceNotFound) {
		return err
	}
	return err
}
