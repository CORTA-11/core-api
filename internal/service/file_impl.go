package service

import (
	"context"
	"io"

	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/minio/minio-go/v7"
)

var ErrInvalidFileName = tenancy.ErrInvalidStorageName

type fileService struct {
	client     *minio.Client
	bucketName string
}

func NewFileService(client *minio.Client, bucketName string) FileService {
	return &fileService{
		client:     client,
		bucketName: bucketName,
	}
}

func (f *fileService) UploadFile(ctx context.Context, team tenancy.TeamContext, fileName string, file io.Reader) error {
	key, err := tenancy.StorageObjectKey(team, fileName)
	if err != nil {
		return err
	}

	_, err = f.client.PutObject(ctx, f.bucketName, key, file, -1, minio.PutObjectOptions{})
	return err
}

func (f *fileService) DownloadFile(ctx context.Context, team tenancy.TeamContext, fileName string, file io.Writer) error {
	key, err := tenancy.StorageObjectKey(team, fileName)
	if err != nil {
		return err
	}

	object, err := f.client.GetObject(ctx, f.bucketName, key, minio.GetObjectOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = object.Close() }()

	_, err = io.Copy(file, object)
	return err
}
