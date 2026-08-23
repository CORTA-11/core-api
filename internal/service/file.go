package service

import (
	"context"
	"io"

	"github.com/CORTA-11/core-api/internal/tenancy"
)

type FileService interface {
	UploadFile(ctx context.Context, team tenancy.TeamContext, fileName string, file io.Reader) error
	DownloadFile(ctx context.Context, team tenancy.TeamContext, fileName string, file io.Writer) error
}
