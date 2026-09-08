//go:build isolation

package integration_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/service"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/CORTA-11/core-api/internal/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	miniogo "github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A member who joins after the team's files were uploaded cannot read the
// earlier key versions (the wraps do not cover them). They request access and
// the team leader approves after re-wrapping each historical version for them.
func TestE2EKeyAccessRequestAndGrant(t *testing.T) {
	fixture := newTenantBoundaryFixture(t)
	ctx := context.Background()

	owner := session.Principal{UserID: fixture.users.shared, SessionID: uuid.New()}
	alpha := session.Principal{UserID: fixture.users.alpha, SessionID: uuid.New()}
	latecomer := session.Principal{UserID: fixture.users.beta, SessionID: uuid.New()}

	org := fixture.orgs[0]
	team := org.teams[0]

	// Owner is the team leader; beta joins the team only after the shared keys
	// rotate, so beta holds no wrap for the early versions.
	membersTable := pgx.Identifier{org.schema, "team_members"}.Sanitize()
	_, err := fixture.adminPool.Exec(ctx, `UPDATE `+membersTable+` SET role = 'team_admin' WHERE team_id = $1 AND user_public_id = $2`, team.id, owner.UserID)
	require.NoError(t, err)
	// The latecomer joins the organization, then the team later, after the first
	// key version already exists.
	_, err = fixture.adminPool.Exec(ctx, `INSERT INTO public.org_user (org_id, user_id)
		SELECT $1, id FROM public.users WHERE user_id = $2`, org.id, latecomer.UserID)
	require.NoError(t, err)
	_, err = fixture.adminPool.Exec(ctx, `INSERT INTO `+membersTable+`
		(team_id, user_public_id, role) VALUES ($1, $2, 'viewer')`, team.id, latecomer.UserID)
	require.NoError(t, err)

	authorizer := authorization.NewAuthorizer(fixture.resolver, fixture.executor)
	keySvc := service.NewKeyService(fixture.adminPool, authorizer)
	keyAccess := service.NewKeyAccessApplication(authorizer)

	minioClient := testsupport.OpenMinIO(t)
	bucket := "integration-keyaccess-files"
	exists, err := minioClient.BucketExists(ctx, bucket)
	require.NoError(t, err)
	if !exists {
		require.NoError(t, minioClient.MakeBucket(ctx, bucket, miniogo.MakeBucketOptions{}))
	}
	t.Cleanup(func() {
		testsupport.EmptyBucket(t, minioClient, bucket)
		_ = minioClient.RemoveBucket(context.Background(), bucket)
	})
	fileSvc := service.NewFileService(minioClient, bucket, authorizer)

	ownerPublicKey := "owner-public-key-ssh-rsa-AAAAB3NzaC1yc2EAAAADAQABAAABgQCxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	alphaPublicKey := "alpha-public-key-ssh-rsa-AAAAB3NzaC1yc2EAAAADAQABAAABgQCxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	latePublicKey := "late-public-key-ssh-rsa-AAAAB3NzaC1yc2EAAAADAQABAAABgQCxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	ownerPrivate := "encrypted-owner-private-key-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	latePrivate := "encrypted-late-private-key-CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC"
	salt := "a-fixed-salt"

	_, err = keySvc.UpsertUserKeys(ctx, owner, service.UserKeyUpdate{
		PublicKey: ownerPublicKey, EncryptedPrivateKey: ptr(ownerPrivate),
		KEKSalt: ptr(salt), KEKIterations: ptr(int32(600000)), KEKAlgorithm: ptr("pbkdf2-sha256"),
	})
	require.NoError(t, err)
	_, err = keySvc.UpsertUserKeys(ctx, alpha, service.UserKeyUpdate{
		PublicKey: alphaPublicKey, EncryptedPrivateKey: ptr("encrypted-alpha-private-key-BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"),
		KEKSalt: ptr(salt), KEKIterations: ptr(int32(600000)), KEKAlgorithm: ptr("pbkdf2-sha256"),
	})
	require.NoError(t, err)
	_, err = keySvc.UpsertUserKeys(ctx, latecomer, service.UserKeyUpdate{
		PublicKey: latePublicKey, EncryptedPrivateKey: ptr(latePrivate),
		KEKSalt: ptr(salt), KEKIterations: ptr(int32(600000)), KEKAlgorithm: ptr("pbkdf2-sha256"),
	})
	require.NoError(t, err)

	// The first key version existed before beta joined and covers owner+alpha.
	created, err := keySvc.CreateTeamKey(ctx, owner, org.publicID, team.publicID, service.TeamKeyVersionInput{Wraps: []service.TeamKeyWrap{
		{UserID: owner.UserID, Key: "team-key-v1-owner-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", Algorithm: "rsa-oaep-2048"},
		{UserID: alpha.UserID, Key: "team-key-v1-alpha-BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB", Algorithm: "rsa-oaep-2048"},
	}})
	require.NoError(t, err)
	assert.Equal(t, int32(1), created.Version)

	payload := "encrypted-payload-data"
	fileMeta, err := fileSvc.UploadFile(ctx, owner, org.publicID, team.publicID,
		"secret_report.enc", "application/octet-stream",
		strings.NewReader(payload), int64(len(payload)), []byte("aes-iv-bytes-16"), created.Version)
	require.NoError(t, err)

	// The latecomer cannot see the earlier key version, so download is impossible.
	hidden, err := keySvc.ListTeamKeys(ctx, latecomer, org.publicID, team.publicID)
	require.NoError(t, err)
	assert.Empty(t, hidden)

	// The latecomer requests access; a second request while one is pending collides.
	request, err := keyAccess.CreateKeyAccessRequest(ctx, latecomer, org.publicID, team.publicID)
	require.NoError(t, err)
	assert.Equal(t, "pending", request.Status)
	assert.Equal(t, latecomer.UserID, request.RequestedBy)
	_, err = keyAccess.CreateKeyAccessRequest(ctx, latecomer, org.publicID, team.publicID)
	require.ErrorIs(t, err, service.ErrConflict)

	// A non-leader member cannot decide.
	_, err = keyAccess.DecideKeyAccessRequest(ctx, alpha, org.publicID, team.publicID, request.ID, "granted")
	require.Error(t, err)

	// The leader re-wraps the historical version for the latecomer, then approves.
	rewrapped, err := keySvc.AddTeamKeyMemberWrap(ctx, owner, org.publicID, team.publicID, 1, service.TeamKeyWrap{
		UserID: latecomer.UserID, Key: "re-wrapped-team-key-for-beta-CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC", Algorithm: "rsa-oaep-2048",
	})
	require.NoError(t, err)
	assert.Equal(t, int32(1), rewrapped.Version)
	assert.Contains(t, rewrapped.WrappedUserIDs, latecomer.UserID)

	decided, err := keyAccess.DecideKeyAccessRequest(ctx, owner, org.publicID, team.publicID, request.ID, "granted")
	require.NoError(t, err)
	assert.Equal(t, "granted", decided.Status)
	assert.NotNil(t, decided.DecidedBy)
	assert.Equal(t, owner.UserID, *decided.DecidedBy)

	// The latecomer now lists the version and decrypts the old file.
	visible, err := keySvc.ListTeamKeys(ctx, latecomer, org.publicID, team.publicID)
	require.NoError(t, err)
	require.Len(t, visible, 1)
	require.Equal(t, int32(1), visible[0].Version)
	require.Len(t, visible[0].Wraps, 1)
	assert.Equal(t, latecomer.UserID, visible[0].Wraps[0].UserID)

	downMeta, stream, err := fileSvc.DownloadFile(ctx, latecomer, org.publicID, team.publicID, fileMeta.ID)
	require.NoError(t, err)
	defer stream.Close()
	var downloaded bytes.Buffer
	_, err = downloaded.ReadFrom(stream)
	require.NoError(t, err)
	assert.Equal(t, downMeta.ID, fileMeta.ID)
	assert.Equal(t, payload, downloaded.String())

	// Re-deciding the settled request conflicts.
	_, err = keyAccess.DecideKeyAccessRequest(ctx, owner, org.publicID, team.publicID, request.ID, "denied")
	require.ErrorIs(t, err, service.ErrConflict)
}
