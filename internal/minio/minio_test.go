package minio

import (
	"context"
	"errors"
	"testing"

	miniogo "github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeBucketClient struct {
	makeErr   error
	exists    bool
	existsErr error
	makeCalls int
}

func (client *fakeBucketClient) MakeBucket(context.Context, string, miniogo.MakeBucketOptions) error {
	client.makeCalls++
	return client.makeErr
}

func (client *fakeBucketClient) BucketExists(context.Context, string) (bool, error) {
	return client.exists, client.existsErr
}

func TestEnsureBucketIsIdempotent(t *testing.T) {
	client := &fakeBucketClient{makeErr: errors.New("already exists"), exists: true}
	require.NoError(t, EnsureBucket(context.Background(), client, "files"))
	assert.Equal(t, 1, client.makeCalls)
}

func TestEnsureBucketReturnsProvisioningFailure(t *testing.T) {
	want := errors.New("permission denied")
	client := &fakeBucketClient{makeErr: want}
	err := EnsureBucket(context.Background(), client, "files")
	require.Error(t, err)
	assert.ErrorIs(t, err, want)
}

func TestVerifyBucketRequiresExistingBucket(t *testing.T) {
	err := VerifyBucket(context.Background(), &fakeBucketClient{}, "files")
	require.Error(t, err)
	assert.ErrorContains(t, err, "make bootstrap")
}

func TestNewClientReturnsInvalidEndpointError(t *testing.T) {
	_, err := NewClient("http://bad endpoint", "access", "secret", false)
	require.Error(t, err)
}
