package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

var ErrInvalidFileName = errors.New("invalid file name")

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

func (f *fileService) UploadFile(ctx context.Context, orgID uuid.UUID, teamID int, fileName string, file io.Reader) error {
	key, err := fileObjectKey(orgID, teamID, fileName)
	if err != nil {
		return err
	}

	_, err = f.client.PutObject(ctx, f.bucketName, key, file, -1, minio.PutObjectOptions{})
	return err
}

func (f *fileService) DownloadFile(ctx context.Context, orgID uuid.UUID, teamID int, fileName string, file io.Writer) error {
	key, err := fileObjectKey(orgID, teamID, fileName)
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

func fileObjectKey(orgID uuid.UUID, teamID int, fileName string) (string, error) {
	if fileName == "" || fileName == "." || fileName == ".." ||
		strings.ContainsAny(fileName, `/\\`) || strings.ContainsRune(fileName, '\x00') {
		return "", ErrInvalidFileName
	}

	return fmt.Sprintf("orgs/%s/teams/%s/files/%s", orgID.String(), strconv.Itoa(teamID), fileName), nil
}
