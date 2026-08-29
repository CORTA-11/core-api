package minio

import (
	"context"
	"fmt"

	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type BucketClient interface {
	MakeBucket(context.Context, string, miniogo.MakeBucketOptions) error
	BucketExists(context.Context, string) (bool, error)
}

// NewClient creates a client.
func NewClient(endpoint, accessKeyID, secretAccessKey string, useSSL bool) (*miniogo.Client, error) {
	client, err := miniogo.New(endpoint, &miniogo.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create MinIO client: %w", err)
	}
	return client, nil
}

// EnsureBucket ensures bucket.
func EnsureBucket(ctx context.Context, client BucketClient, bucketName string) error {
	if err := client.MakeBucket(ctx, bucketName, miniogo.MakeBucketOptions{}); err != nil {
		exists, existsErr := client.BucketExists(ctx, bucketName)
		if existsErr != nil {
			return fmt.Errorf("check MinIO bucket after create failure: %w", existsErr)
		}
		if !exists {
			return fmt.Errorf("create MinIO bucket: %w", err)
		}
	}
	return nil
}

// VerifyBucket verifies bucket.
func VerifyBucket(ctx context.Context, client BucketClient, bucketName string) error {
	exists, err := client.BucketExists(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("check MinIO bucket: %w", err)
	}
	if !exists {
		return fmt.Errorf("configured MinIO bucket %q does not exist; run make bootstrap", bucketName)
	}
	return nil
}
