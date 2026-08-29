package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/invitation"
	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InvitationView struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type InvitationCreatedView struct {
	InvitationView
	Token string `json:"token"`
}

type InvitationPreview struct {
	OrganizationID   uuid.UUID `json:"organization_id"`
	OrganizationName string    `json:"organization_name"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type InvitationApplication struct {
	pool    *pgxpool.Pool
	binding *invitation.Binding
	random  io.Reader
}

// NewInvitationApplication creates an invitation application.
func NewInvitationApplication(pool *pgxpool.Pool, binding *invitation.Binding) *InvitationApplication {
	return &InvitationApplication{pool: pool, binding: binding, random: rand.Reader}
}

// Create creates the requested resource.
func (application *InvitationApplication) Create(ctx context.Context, principal session.Principal, orgID uuid.UUID, email string) (InvitationCreatedView, error) {
	if application == nil || application.pool == nil || application.binding == nil || !validPrincipal(principal) {
		return InvitationCreatedView{}, authorization.ErrUnauthenticated
	}
	fingerprint, err := application.binding.EmailFingerprint(email)
	if err != nil {
		return InvitationCreatedView{}, ErrInvalidInput
	}
	token, tokenHash, err := application.binding.GenerateToken(application.random)
	if err != nil {
		return InvitationCreatedView{}, fmt.Errorf("generate invitation token: %w", err)
	}
	tx, err := application.pool.Begin(ctx)
	if err != nil {
		return InvitationCreatedView{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := publicdb.New(tx)
	if _, err = queries.PurgeExpiredOrganizationInvitations(ctx, 100); err != nil {
		return InvitationCreatedView{}, err
	}
	admin, err := queries.LockOrganizationInvitationAdminContext(ctx, publicdb.LockOrganizationInvitationAdminContextParams{OrganizationPublicID: orgID, UserPublicID: principal.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return InvitationCreatedView{}, authorization.ErrResourceNotFound
	}
	if err != nil {
		return InvitationCreatedView{}, err
	}
	if admin.LifecycleState != "active" || admin.OwnerCount < 1 {
		return InvitationCreatedView{}, authorization.ErrOperationDenied
	}
	if admin.Role != "owner" && admin.Role != "administrator" {
		return InvitationCreatedView{}, authorization.ErrOperationDenied
	}
	row, err := queries.CreateOrganizationInvitation(ctx, publicdb.CreateOrganizationInvitationParams{OrgID: admin.OrgID, EmailHmac: fingerprint, TokenHash: tokenHash})
	if err != nil {
		return InvitationCreatedView{}, classifyConflict(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return InvitationCreatedView{}, err
	}
	return InvitationCreatedView{InvitationView: InvitationView{ID: row.PublicID, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt}, Token: token}, nil
}

// Preview handles the preview operation.
func (application *InvitationApplication) Preview(ctx context.Context, token string) (InvitationPreview, error) {
	_, hash, err := application.binding.ParseToken(token)
	if err != nil {
		return InvitationPreview{}, authorization.ErrResourceNotFound
	}
	row, err := publicdb.New(application.pool).GetOrganizationInvitationByTokenHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return InvitationPreview{}, authorization.ErrResourceNotFound
	}
	if err != nil {
		return InvitationPreview{}, err
	}
	return InvitationPreview{OrganizationID: row.OrganizationPublicID, OrganizationName: row.OrganizationName, ExpiresAt: row.ExpiresAt}, nil
}

// List lists the requested resources.
func (application *InvitationApplication) List(ctx context.Context, principal session.Principal, orgID uuid.UUID) ([]InvitationView, error) {
	tx, err := application.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := publicdb.New(tx)
	admin, err := queries.LockOrganizationInvitationAdminContext(ctx, publicdb.LockOrganizationInvitationAdminContextParams{OrganizationPublicID: orgID, UserPublicID: principal.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, authorization.ErrResourceNotFound
	}
	if err != nil {
		return nil, err
	}
	if admin.Role != "owner" && admin.Role != "administrator" {
		return nil, authorization.ErrOperationDenied
	}
	rows, err := queries.ListOrganizationInvitationsAfter(ctx, publicdb.ListOrganizationInvitationsAfterParams{OrgID: admin.OrgID, AfterCreatedAt: time.Time{}, AfterPublicID: uuid.Nil, Limit: 101})
	if err != nil {
		return nil, err
	}
	views := make([]InvitationView, 0, len(rows))
	for _, row := range rows {
		views = append(views, InvitationView{ID: row.PublicID, CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt})
	}
	return views, tx.Commit(ctx)
}

// Revoke handles the revoke operation.
func (application *InvitationApplication) Revoke(ctx context.Context, principal session.Principal, orgID, invitationID uuid.UUID) error {
	tx, err := application.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := publicdb.New(tx)
	admin, err := queries.LockOrganizationInvitationAdminContext(ctx, publicdb.LockOrganizationInvitationAdminContextParams{OrganizationPublicID: orgID, UserPublicID: principal.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return authorization.ErrResourceNotFound
	}
	if err != nil {
		return err
	}
	if admin.LifecycleState != "active" || admin.OwnerCount < 1 || (admin.Role != "owner" && admin.Role != "administrator") {
		return authorization.ErrOperationDenied
	}
	_, err = queries.DeleteOrganizationInvitation(ctx, publicdb.DeleteOrganizationInvitationParams{PublicID: invitationID, OrgID: admin.OrgID})
	if errors.Is(err, pgx.ErrNoRows) {
		return authorization.ErrResourceNotFound
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Consume handles the consume operation.
func (application *InvitationApplication) Consume(ctx context.Context, principal session.Principal, token string, accept bool) error {
	if !validPrincipal(principal) {
		return authorization.ErrUnauthenticated
	}
	_, hash, err := application.binding.ParseToken(token)
	if err != nil {
		return authorization.ErrResourceNotFound
	}
	tx, err := application.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	queries := publicdb.New(tx)
	row, err := queries.GetOrganizationInvitationByTokenHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return authorization.ErrResourceNotFound
	}
	if err != nil {
		return err
	}
	user, err := queries.GetActiveUserInvitationIdentity(ctx, principal.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return authorization.ErrResourceNotFound
	}
	if err != nil {
		return err
	}
	fingerprint, err := application.binding.EmailFingerprint(user.EmailCanonical)
	if err != nil || !hmac.Equal(fingerprint, row.EmailHmac) {
		return authorization.ErrResourceNotFound
	}
	if accept {
		_, err = queries.AcceptOrganizationInvitation(ctx, publicdb.AcceptOrganizationInvitationParams{InvitationID: row.ID, UserID: user.ID})
	} else {
		_, err = queries.DeclineOrganizationInvitation(ctx, row.ID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return authorization.ErrResourceNotFound
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Cleanup handles the cleanup operation.
func (application *InvitationApplication) Cleanup(ctx context.Context) (int64, error) {
	return publicdb.New(application.pool).PurgeExpiredOrganizationInvitations(ctx, 100)
}
