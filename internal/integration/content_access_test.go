//go:build isolation

package integration_test

import (
	"context"
	"testing"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestContentCreatorControlsAccessAndDeniedRequestsCanRetry(t *testing.T) {
	f := newTenantBoundaryFixture(t)
	ctx := context.Background()
	org, team := f.orgs[0], f.orgs[0].teams[0]
	creator := session.Principal{UserID: f.users.shared, SessionID: uuid.New()}
	member := session.Principal{UserID: f.users.alpha, SessionID: uuid.New()}
	outsider := session.Principal{UserID: f.users.outsider, SessionID: uuid.New()}
	auth := authorization.NewAuthorizer(f.resolver, f.executor)
	docs := service.NewDocumentApplication(auth, []byte("test-document-secret-1234567890123456"), contentTestCloser{})
	access := service.NewContentAccessApplication(auth)
	// Team administrators still need the individual creator's permission.
	_, err := f.adminPool.Exec(ctx, "UPDATE "+pgx.Identifier{org.schema, "team_members"}.Sanitize()+" SET role = 'team_admin' WHERE team_id = $1 AND user_public_id = $2", team.id, member.UserID)
	require.NoError(t, err)
	doc, err := docs.Create(ctx, creator, org.publicID, team.publicID, "Private notes")
	require.NoError(t, err)
	_, err = docs.Get(ctx, creator, org.publicID, team.publicID, doc.ID)
	require.NoError(t, err)
	_, err = docs.Get(ctx, member, org.publicID, team.publicID, doc.ID)
	require.ErrorIs(t, err, authorization.ErrResourceNotFound)
	_, err = docs.IssueSocketTicket(ctx, member, "Member", org.publicID, team.publicID, doc.ID)
	require.ErrorIs(t, err, authorization.ErrResourceNotFound)
	_, err = docs.LoadState(ctx, member.UserID, org.publicID, team.publicID, doc.ID)
	require.ErrorIs(t, err, authorization.ErrResourceNotFound)
	body := "secret"
	_, err = docs.Update(ctx, member, org.publicID, team.publicID, doc.ID, service.DocumentPatch{BodyHTML: &body})
	require.ErrorIs(t, err, authorization.ErrResourceNotFound)
	require.NoError(t, access.Request(ctx, member, org.publicID, team.publicID, "document", doc.ID))
	snapshot, err := access.List(ctx, creator, org.publicID, team.publicID)
	require.NoError(t, err)
	require.Len(t, snapshot.Requests, 1)
	requestID := snapshot.Requests[0].PublicID
	require.ErrorIs(t, access.Request(ctx, member, org.publicID, team.publicID, "document", doc.ID), authorization.ErrResourceNotFound)
	// A third current member sees the catalog, not another member's request.
	_, err = f.adminPool.Exec(ctx, "INSERT INTO public.org_user (org_id, user_id) SELECT $1, id FROM public.users WHERE user_id = $2", org.id, f.users.beta)
	require.NoError(t, err)
	_, err = f.adminPool.Exec(ctx, "INSERT INTO "+pgx.Identifier{org.schema, "team_members"}.Sanitize()+" (team_id, user_public_id, role) VALUES ($1, $2, 'viewer')", team.id, f.users.beta)
	require.NoError(t, err)
	third := session.Principal{UserID: f.users.beta, SessionID: uuid.New()}
	private, err := access.List(ctx, third, org.publicID, team.publicID)
	require.NoError(t, err)
	require.Len(t, private.Items, 1)
	require.Empty(t, private.Requests)
	require.False(t, private.Items[0].CanAccess)
	require.ErrorIs(t, access.Decide(ctx, member, org.publicID, team.publicID, requestID, "granted"), authorization.ErrResourceNotFound)
	require.NoError(t, access.Decide(ctx, creator, org.publicID, team.publicID, requestID, "denied"))
	require.NoError(t, access.Request(ctx, member, org.publicID, team.publicID, "document", doc.ID))
	require.NoError(t, access.Decide(ctx, creator, org.publicID, team.publicID, requestID, "granted"))
	require.ErrorIs(t, access.Request(ctx, member, org.publicID, team.publicID, "document", doc.ID), authorization.ErrResourceNotFound)
	require.ErrorIs(t, access.Decide(ctx, creator, org.publicID, team.publicID, requestID, "denied"), authorization.ErrResourceNotFound)
	_, err = docs.Get(ctx, member, org.publicID, team.publicID, doc.ID)
	require.NoError(t, err)
	_, err = docs.Update(ctx, member, org.publicID, team.publicID, doc.ID, service.DocumentPatch{BodyHTML: &body})
	require.NoError(t, err)
	// Editing never transfers creator authority.
	snapshot, err = access.List(ctx, member, org.publicID, team.publicID)
	require.NoError(t, err)
	require.Equal(t, creator.UserID, snapshot.Items[0].CreatorID)
	require.ErrorIs(t, access.Grant(ctx, member, org.publicID, team.publicID, "document", doc.ID, creator.UserID), authorization.ErrResourceNotFound)
	_, err = access.List(ctx, outsider, org.publicID, team.publicID)
	require.Error(t, err)
	require.Error(t, access.Request(ctx, member, f.orgs[1].publicID, f.orgs[1].teams[0].publicID, "document", doc.ID))
	require.ErrorIs(t, access.Grant(ctx, creator, org.publicID, org.teams[1].publicID, "document", doc.ID, member.UserID), authorization.ErrResourceNotFound)

	fileID := uuid.New()
	files := pgx.Identifier{org.schema, "files"}.Sanitize()
	_, err = f.adminPool.Exec(ctx, "INSERT INTO "+files+` (public_id, team_id, name, size, content_type, object_key, iv, key_version, uploaded_by)
 VALUES ($1, $2, 'private.txt', 16, 'application/octet-stream', 'test-object', '\x00', 1, $3)`, fileID, team.id, creator.UserID)
	require.NoError(t, err)
	getFile := func(p session.Principal) error {
		return auth.WithinTeam(ctx, p, org.publicID, team.publicID, authorization.PermissionFileRead, func(q *tenantdb.Queries) error {
			_, err := q.GetFileByID(ctx, tenantdb.GetFileByIDParams{TeamID: team.id, PublicID: fileID})
			return err
		})
	}
	require.ErrorIs(t, getFile(member), pgx.ErrNoRows)
	require.NoError(t, getFile(creator))
	require.NoError(t, access.Grant(ctx, creator, org.publicID, team.publicID, "file", fileID, member.UserID))
	require.NoError(t, getFile(member))
	require.ErrorIs(t, access.Grant(ctx, creator, org.publicID, team.publicID, "file", fileID, outsider.UserID), authorization.ErrResourceNotFound)
	// Creators can still deny stale pending requests after the requester leaves.
	pendingDoc, err := docs.Create(ctx, creator, org.publicID, team.publicID, "Pending notes")
	require.NoError(t, err)
	require.NoError(t, access.Request(ctx, member, org.publicID, team.publicID, "document", pendingDoc.ID))
	pending, err := access.List(ctx, creator, org.publicID, team.publicID)
	require.NoError(t, err)
	var staleRequest uuid.UUID
	for _, request := range pending.Requests {
		if request.ResourceID == pendingDoc.ID {
			staleRequest = request.PublicID
		}
	}
	require.NotEqual(t, uuid.Nil, staleRequest)
	// Membership remains mandatory after approval.
	_, err = f.adminPool.Exec(ctx, "DELETE FROM "+pgx.Identifier{org.schema, "team_members"}.Sanitize()+" WHERE team_id = $1 AND user_public_id = $2", team.id, member.UserID)
	require.NoError(t, err)
	_, err = docs.Get(ctx, member, org.publicID, team.publicID, doc.ID)
	require.Error(t, err)
	require.Error(t, getFile(member))
	require.NoError(t, access.Decide(ctx, creator, org.publicID, team.publicID, staleRequest, "denied"))
}

type contentTestCloser struct{}

func (contentTestCloser) CloseDocumentRoom(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return nil
}
