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

func TestE2EEKeysAndFilesRoundTrip(t *testing.T) {
	fixture := newTenantBoundaryFixture(t)
	ctx := context.Background()

	owner := session.Principal{UserID: fixture.users.shared, SessionID: uuid.New()}
	member := session.Principal{UserID: fixture.users.alpha, SessionID: uuid.New()}
	outsider := session.Principal{UserID: fixture.users.outsider, SessionID: uuid.New()}

	org := fixture.orgs[0]
	team := org.teams[0]

	// Update team roles so that owner is a team_admin and member is a contributor
	membersTable := pgx.Identifier{org.schema, "team_members"}.Sanitize()
	_, err := fixture.adminPool.Exec(ctx, `UPDATE `+membersTable+` SET role = 'team_admin' WHERE team_id = $1 AND user_public_id = $2`, team.id, owner.UserID)
	require.NoError(t, err)
	_, err = fixture.adminPool.Exec(ctx, `UPDATE `+membersTable+` SET role = 'contributor' WHERE team_id = $1 AND user_public_id = $2`, team.id, member.UserID)
	require.NoError(t, err)

	// 1. Initialize E2EE Services
	authorizer := authorization.NewAuthorizer(fixture.resolver, fixture.executor)
	keySvc := service.NewKeyService(fixture.adminPool, authorizer)

	minioClient := testsupport.OpenMinIO(t)
	bucket := "integration-e2ee-files"
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

	// 2. Register per-user key pairs: RSA public key plus a private key sealed
	// with a password-derived key (PBKDF2-SHA256). The server never sees either
	// in the clear, but this test only exercises the storage contract.
	ownerPublicKey := "owner-public-key-ssh-rsa-AAAAB3NzaC1yc2EAAAADAQABAAABgQCxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	memberPublicKey := "member-public-key-ssh-rsa-AAAAB3NzaC1yc2EAAAADAQABAAABgQCxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	ownerPrivate := "encrypted-owner-private-key-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	memberPrivate := "encrypted-member-private-key-BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	salt := "a-fixed-salt"

	resKey, err := keySvc.UpsertUserKeys(ctx, owner, service.UserKeyUpdate{
		PublicKey:           ownerPublicKey,
		EncryptedPrivateKey: ptr(ownerPrivate),
		KEKSalt:             ptr(salt),
		KEKIterations:       ptr(int32(600000)),
		KEKAlgorithm:        ptr("pbkdf2-sha256"),
	})
	require.NoError(t, err)
	assert.Equal(t, ownerPublicKey, resKey.PublicKey)
	require.NotNil(t, resKey.EncryptedPrivateKey)
	assert.Equal(t, ownerPrivate, *resKey.EncryptedPrivateKey)

	resKey2, err := keySvc.UpsertUserKeys(ctx, member, service.UserKeyUpdate{
		PublicKey:           memberPublicKey,
		EncryptedPrivateKey: ptr(memberPrivate),
		KEKSalt:             ptr(salt),
		KEKIterations:       ptr(int32(600000)),
		KEKAlgorithm:        ptr("pbkdf2-sha256"),
	})
	require.NoError(t, err)
	assert.Equal(t, memberPublicKey, resKey2.PublicKey)

	// 3. Retrieve own key material
	gotKey, err := keySvc.GetUserKeys(ctx, member)
	require.NoError(t, err)
	assert.Equal(t, memberPublicKey, gotKey.PublicKey)
	require.NotNil(t, gotKey.EncryptedPrivateKey)
	assert.Equal(t, memberPrivate, *gotKey.EncryptedPrivateKey)

	// 4. Retrieve Public Keys of Team Members
	teamKeys, err := keySvc.GetPublicKeysForTeam(ctx, owner, org.publicID, team.publicID)
	require.NoError(t, err)
	assert.Len(t, teamKeys, 2) // Owner and Member are in team

	// 5. Create Team Shared Key (Symmetric key encrypted with each member's public key)
	sharedKeys := []service.TeamKeyWrap{
		{UserID: owner.UserID, Key: "encrypted-team-key-for-owner-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", Algorithm: "rsa-oaep-2048"},
		{UserID: member.UserID, Key: "encrypted-team-key-for-member-BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB", Algorithm: "rsa-oaep-2048"},
	}
	created, err := keySvc.CreateTeamKey(ctx, owner, org.publicID, team.publicID, service.TeamKeyVersionInput{Wraps: sharedKeys})
	require.NoError(t, err)
	assert.Equal(t, int32(1), created.Version)
	assert.Equal(t, "active", created.Status)
	assert.Equal(t, "aes-256-gcm", created.Algorithm)
	require.Len(t, created.WrappedUserIDs, 2)
	assert.Contains(t, created.WrappedUserIDs, owner.UserID)
	assert.Contains(t, created.WrappedUserIDs, member.UserID)
	// Wraps are filtered to the caller
	require.Len(t, created.Wraps, 1)
	assert.Equal(t, owner.UserID, created.Wraps[0].UserID)

	// 6. Get/List Shared Keys for User (only wraps of the caller are exposed)
	callerKeys, err := keySvc.ListTeamKeys(ctx, member, org.publicID, team.publicID)
	require.NoError(t, err)
	require.Len(t, callerKeys, 1)
	assert.Equal(t, int32(1), callerKeys[0].Version)
	require.Len(t, callerKeys[0].Wraps, 1)
	assert.Equal(t, member.UserID, callerKeys[0].Wraps[0].UserID)
	assert.Len(t, callerKeys[0].WrappedUserIDs, 2)

	// 7. Upload Encrypted File (bound to the active team key version)
	payload := "encrypted-payload-data"
	iv := []byte("aes-iv-bytes-16")
	fileMeta, err := fileSvc.UploadFile(
		ctx,
		owner,
		org.publicID,
		team.publicID,
		"secret_report.enc",
		"application/octet-stream",
		strings.NewReader(payload),
		int64(len(payload)),
		iv,
		created.Version,
	)
	require.NoError(t, err)
	assert.Equal(t, "secret_report.enc", fileMeta.Name)
	assert.Equal(t, int64(len(payload)), fileMeta.Size)
	assert.Equal(t, iv, fileMeta.IV)
	assert.Equal(t, int32(1), fileMeta.KeyVersion)

	// 8. Outsider Should Not List/Download Files
	_, err = fileSvc.ListFiles(ctx, outsider, org.publicID, team.publicID)
	assert.Error(t, err) // RLS/auth rejection

	// 9. Member Lists Files
	files, err := fileSvc.ListFiles(ctx, member, org.publicID, team.publicID)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, fileMeta.ID, files[0].ID)

	// 10. Member Downloads File
	downMeta, stream, err := fileSvc.DownloadFile(ctx, member, org.publicID, team.publicID, fileMeta.ID)
	require.NoError(t, err)
	defer stream.Close()

	assert.Equal(t, fileMeta.ID, downMeta.ID)
	assert.Equal(t, "secret_report.enc", downMeta.Name)

	var downloadedData bytes.Buffer
	_, err = downloadedData.ReadFrom(stream)
	require.NoError(t, err)
	assert.Equal(t, payload, downloadedData.String())

	// 11. Rotate: member leaves, so a fresh symmetric key is wrapped only for the
	// owner. The previous version must be superseded and hidden from the leaver.
	rotated, err := keySvc.CreateTeamKey(ctx, owner, org.publicID, team.publicID, service.TeamKeyVersionInput{Wraps: []service.TeamKeyWrap{
		{UserID: owner.UserID, Key: "rotated-team-key-for-owner-CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC", Algorithm: "rsa-oaep-2048"},
	}})
	require.NoError(t, err)
	assert.Equal(t, int32(2), rotated.Version)
	assert.Equal(t, "active", rotated.Status)
	require.Len(t, rotated.WrappedUserIDs, 1)
	assert.Equal(t, owner.UserID, rotated.WrappedUserIDs[0])

	ownerKeys, err := keySvc.ListTeamKeys(ctx, owner, org.publicID, team.publicID)
	require.NoError(t, err)
	require.Len(t, ownerKeys, 2)
	assert.Equal(t, int32(2), ownerKeys[0].Version)
	assert.Equal(t, "active", ownerKeys[0].Status)
	assert.Equal(t, int32(1), ownerKeys[1].Version)
	assert.Equal(t, "superseded", ownerKeys[1].Status)

	// 12. Owner Deletes File
	err = fileSvc.DeleteFile(ctx, owner, org.publicID, team.publicID, fileMeta.ID)
	require.NoError(t, err)

	// 13. File Should No Longer Exist
	filesAfterDelete, err := fileSvc.ListFiles(ctx, member, org.publicID, team.publicID)
	require.NoError(t, err)
	assert.Empty(t, filesAfterDelete)
}

func ptr[T any](value T) *T {
	return &value
}
