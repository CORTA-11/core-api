package service

import (
	"context"
	"io"

	"github.com/google/uuid"
)

type FileService interface {
	UploadFile(ctx context.Context, orgID uuid.UUID, teamID int, fileName string, file io.Reader) error
	DownloadFile(ctx context.Context, orgID uuid.UUID, teamID int, fileName string, file io.Writer) error
}
