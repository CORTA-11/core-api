package service

import (
	"context"
	"io"
	"time"

	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
)

type FileView struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Size        int64     `json:"size"`
	ContentType string    `json:"content_type"`
	IV          []byte    `json:"iv"`
	KeyVersion  int32     `json:"key_version"`
	UploadedBy  uuid.UUID `json:"uploaded_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type FileService interface {
	UploadFile(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID, name string, contentType string, reader io.Reader, size int64, iv []byte, keyVersion int32) (*FileView, error)
	DownloadFile(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID, fileID uuid.UUID) (*FileView, io.ReadCloser, error)
	ListFiles(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID) ([]FileView, error)
	DeleteFile(ctx context.Context, p session.Principal, orgID uuid.UUID, teamID uuid.UUID, fileID uuid.UUID) error
}
