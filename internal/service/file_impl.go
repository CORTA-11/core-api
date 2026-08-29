package service

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/repository/tenantdb"
	"github.com/CORTA-11/core-api/internal/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	miniogo "github.com/minio/minio-go/v7"
)

type fileService struct {
	minioClient *miniogo.Client
	bucket      string
	authorizer  applicationAuthorizer
}

// NewFileService creates a new instance of FileService.
func NewFileService(minioClient *miniogo.Client, bucket string, authorizer applicationAuthorizer) FileService {
	return &fileService{
		minioClient: minioClient,
		bucket:      bucket,
		authorizer:  authorizer,
	}
}

func (s *fileService) UploadFile(
	ctx context.Context,
	p session.Principal,
	orgID uuid.UUID,
	teamID uuid.UUID,
	name string,
	contentType string,
	reader io.Reader,
	size int64,
	iv []byte,
	keyVersion int32,
) (*FileView, error) {
	if len(name) == 0 || len(name) > 255 {
		return nil, ErrInvalidInput
	}
	if len(iv) == 0 || len(iv) > 64 {
		return nil, ErrInvalidInput
	}
	if size <= 0 {
		return nil, ErrInvalidInput
	}

	fileID := uuid.New()
	objectKey := fmt.Sprintf("orgs/%s/teams/%s/files/%s", orgID, teamID, fileID)

	var row tenantdb.File
	err := s.authorizer.WithinTeam(ctx, p, orgID, teamID, authorization.PermissionFileUpload, func(queries *tenantdb.Queries) error {
		resolvedTeam, err := queries.ResolveTeamContext(ctx, tenantdb.ResolveTeamContextParams{
			PublicID:     teamID,
			UserPublicID: p.UserID,
		})
		if err != nil {
			return err
		}

		_, err = s.minioClient.PutObject(ctx, s.bucket, objectKey, reader, size, miniogo.PutObjectOptions{
			ContentType: contentType,
		})
		if err != nil {
			return fmt.Errorf("upload to MinIO: %w", err)
		}

		row, err = queries.CreateFile(ctx, tenantdb.CreateFileParams{
			TeamID:      resolvedTeam.ID,
			Name:        name,
			Size:        size,
			ContentType: contentType,
			ObjectKey:   objectKey,
			Iv:          iv,
			KeyVersion:  keyVersion,
			UploadedBy:  p.UserID,
		})
		return err
	})
	if err != nil {
		return nil, err
	}

	return &FileView{
		ID:          row.PublicID,
		Name:        row.Name,
		Size:        row.Size,
		ContentType: row.ContentType,
		IV:          row.Iv,
		KeyVersion:  row.KeyVersion,
		UploadedBy:  row.UploadedBy,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}, nil
}

func (s *fileService) DownloadFile(
	ctx context.Context,
	p session.Principal,
	orgID uuid.UUID,
	teamID uuid.UUID,
	fileID uuid.UUID,
) (*FileView, io.ReadCloser, error) {
	var row tenantdb.File
	err := s.authorizer.WithinTeam(ctx, p, orgID, teamID, authorization.PermissionFileRead, func(queries *tenantdb.Queries) error {
		resolvedTeam, err := queries.ResolveTeamContext(ctx, tenantdb.ResolveTeamContextParams{
			PublicID:     teamID,
			UserPublicID: p.UserID,
		})
		if err != nil {
			return err
		}

		var getErr error
		row, getErr = queries.GetFileByID(ctx, tenantdb.GetFileByIDParams{
			TeamID:   resolvedTeam.ID,
			PublicID: fileID,
		})
		return getErr
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, authorization.ErrResourceNotFound
	}
	if err != nil {
		return nil, nil, err
	}

	object, err := s.minioClient.GetObject(ctx, s.bucket, row.ObjectKey, miniogo.GetObjectOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("fetch from MinIO: %w", err)
	}

	view := &FileView{
		ID:          row.PublicID,
		Name:        row.Name,
		Size:        row.Size,
		ContentType: row.ContentType,
		IV:          row.Iv,
		KeyVersion:  row.KeyVersion,
		UploadedBy:  row.UploadedBy,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}

	return view, object, nil
}

func (s *fileService) ListFiles(
	ctx context.Context,
	p session.Principal,
	orgID uuid.UUID,
	teamID uuid.UUID,
) ([]FileView, error) {
	var rows []tenantdb.File
	err := s.authorizer.WithinTeam(ctx, p, orgID, teamID, authorization.PermissionFileRead, func(queries *tenantdb.Queries) error {
		resolvedTeam, err := queries.ResolveTeamContext(ctx, tenantdb.ResolveTeamContextParams{
			PublicID:     teamID,
			UserPublicID: p.UserID,
		})
		if err != nil {
			return err
		}

		var listErr error
		rows, listErr = queries.ListFilesForTeam(ctx, resolvedTeam.ID)
		return listErr
	})
	if err != nil {
		return nil, err
	}

	views := make([]FileView, len(rows))
	for i, row := range rows {
		views[i] = FileView{
			ID:          row.PublicID,
			Name:        row.Name,
			Size:        row.Size,
			ContentType: row.ContentType,
			IV:          row.Iv,
			KeyVersion:  row.KeyVersion,
			UploadedBy:  row.UploadedBy,
			CreatedAt:    row.CreatedAt,
			UpdatedAt:    row.UpdatedAt,
		}
	}
	return views, nil
}

func (s *fileService) DeleteFile(
	ctx context.Context,
	p session.Principal,
	orgID uuid.UUID,
	teamID uuid.UUID,
	fileID uuid.UUID,
) error {
	return s.authorizer.WithinTeam(ctx, p, orgID, teamID, authorization.PermissionFileDelete, func(queries *tenantdb.Queries) error {
		resolvedTeam, err := queries.ResolveTeamContext(ctx, tenantdb.ResolveTeamContextParams{
			PublicID:     teamID,
			UserPublicID: p.UserID,
		})
		if err != nil {
			return err
		}

		_, err = queries.SoftDeleteFile(ctx, tenantdb.SoftDeleteFileParams{
			TeamID:   resolvedTeam.ID,
			PublicID: fileID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return authorization.ErrResourceNotFound
		}
		return err
	})
}
