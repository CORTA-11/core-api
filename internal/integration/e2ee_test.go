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

	// 2. Register User Public Keys
	ownerKey := "owner-public-key-ssh-rsa-AAAAB3NzaC1yc2EAAAADAQABAAABgQCxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	memberKey := "member-public-key-ssh-rsa-AAAAB3NzaC1yc2EAAAADAQABAAABgQCxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

	resKey, err := keySvc.UpsertPublicKey(ctx, owner, ownerKey)
	require.NoError(t, err)
	assert.Equal(t, ownerKey, resKey.PublicKey)

	resKey2, err := keySvc.UpsertPublicKey(ctx, member, memberKey)
	require.NoError(t, err)
	assert.Equal(t, memberKey, resKey2.PublicKey)

	// 3. Retrieve User Public Key
	gotKey, err := keySvc.GetPublicKey(ctx, member, owner.UserID)
	require.NoError(t, err)
	assert.Equal(t, ownerKey, gotKey.PublicKey)

	// 4. Retrieve Public Keys of Team Members
	teamKeys, err := keySvc.GetPublicKeysForTeam(ctx, owner, org.publicID, team.publicID)
	require.NoError(t, err)
	assert.Len(t, teamKeys, 2) // Owner and Member are in team

	// 5. Share/Upsert Team Shared Keys (Symmetric keys encrypted with each member's public key)
	encryptedKeyForOwner := "enc-key-for-owner"
	encryptedKeyForMember := "enc-key-for-member"
	sharedKeys := []service.TeamSharedKey{
		{UserID: owner.UserID, EncryptedKey: encryptedKeyForOwner, KeyVersion: 1},
		{UserID: member.UserID, EncryptedKey: encryptedKeyForMember, KeyVersion: 1},
	}
	err = keySvc.UpsertTeamSharedKeys(ctx, owner, org.publicID, team.publicID, sharedKeys)
	require.NoError(t, err)

	// 6. Get/List Shared Keys for User
	callerKeys, err := keySvc.ListTeamSharedKeysForUser(ctx, member, org.publicID, team.publicID, member.UserID)
	require.NoError(t, err)
	require.Len(t, callerKeys, 1)
	assert.Equal(t, encryptedKeyForMember, callerKeys[0].EncryptedKey)

	// 7. Upload Encrypted File
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
		1,
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

	// 11. Owner Deletes File
	err = fileSvc.DeleteFile(ctx, owner, org.publicID, team.publicID, fileMeta.ID)
	require.NoError(t, err)

	// 12. File Should No Longer Exist
	filesAfterDelete, err := fileSvc.ListFiles(ctx, member, org.publicID, team.publicID)
	require.NoError(t, err)
	assert.Empty(t, filesAfterDelete)
}
