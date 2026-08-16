package handlers

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CORTA-11/core-api/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubFileService struct {
	uploadFileFn   func(context.Context, uuid.UUID, int, string, io.Reader) error
	downloadFileFn func(context.Context, uuid.UUID, int, string, io.Writer) error
}

func (s *stubFileService) UploadFile(ctx context.Context, orgID uuid.UUID, teamID int, fileName string, file io.Reader) error {
	return s.uploadFileFn(ctx, orgID, teamID, fileName, file)
}

func (s *stubFileService) DownloadFile(ctx context.Context, orgID uuid.UUID, teamID int, fileName string, file io.Writer) error {
	return s.downloadFileFn(ctx, orgID, teamID, fileName, file)
}

type stubFileTeamService struct {
	teamID int
}

func (s *stubFileTeamService) GetTeams(context.Context, string) ([]service.Team, error) {
	return nil, nil
}

func (s *stubFileTeamService) CreateTeam(context.Context, string, string) (*service.Team, error) {
	return nil, nil
}

func (s *stubFileTeamService) GetTeamID(context.Context, string, string) (int, error) {
	return s.teamID, nil
}

func performFileRequest(t *testing.T, fileService service.FileService, method, target, orgID, fileName string) *httptest.ResponseRecorder {
	t.Helper()

	router := chi.NewRouter()
	router.Mount("/{team}/files", fileRouter(&Router{
		teamService: &stubFileTeamService{teamID: 42},
		fileService: fileService,
	}))

	var body bytes.Buffer
	requestTarget := "/platform/files" + target
	request := httptest.NewRequest(method, requestTarget, nil)
	if fileName != "" {
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", fileName)
		require.NoError(t, err)
		_, err = part.Write([]byte("file contents"))
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		request = httptest.NewRequest(method, requestTarget, &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
	}
	if orgID != "" {
		request.Header.Set("X-Org-ID", orgID)
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestUploadFileUsesOrganizationAndTeamScope(t *testing.T) {
	orgID := uuid.MustParse("5dc89da9-f554-43ea-970b-d1ca15e2f921")
	fileService := &stubFileService{
		uploadFileFn: func(_ context.Context, gotOrgID uuid.UUID, gotTeamID int, gotFileName string, file io.Reader) error {
			assert.Equal(t, orgID, gotOrgID)
			assert.Equal(t, 42, gotTeamID)
			assert.Equal(t, "report.pdf", gotFileName)
			contents, err := io.ReadAll(file)
			require.NoError(t, err)
			assert.Equal(t, "file contents", string(contents))
			return nil
		},
	}

	response := performFileRequest(t, fileService, http.MethodPost, "/upload", orgID.String(), "report.pdf")

	assert.Equal(t, http.StatusOK, response.Code)
}

func TestDownloadFileUsesOrganizationAndTeamScope(t *testing.T) {
	orgID := uuid.MustParse("5dc89da9-f554-43ea-970b-d1ca15e2f921")
	fileService := &stubFileService{
		downloadFileFn: func(_ context.Context, gotOrgID uuid.UUID, gotTeamID int, gotFileName string, file io.Writer) error {
			assert.Equal(t, orgID, gotOrgID)
			assert.Equal(t, 42, gotTeamID)
			assert.Equal(t, "report.pdf", gotFileName)
			_, err := file.Write([]byte("downloaded contents"))
			return err
		},
	}

	response := performFileRequest(t, fileService, http.MethodGet, "/download/report.pdf", orgID.String(), "")

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "downloaded contents", response.Body.String())
	assert.Equal(t, "application/octet-stream", response.Header().Get("Content-Type"))
	assert.Equal(t, `attachment; filename=report.pdf`, response.Header().Get("Content-Disposition"))
}

func TestFileRoutesRequireOrganization(t *testing.T) {
	response := performFileRequest(t, &stubFileService{}, http.MethodGet, "/download/report.pdf", "", "")

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Equal(t, "organization id header is missing\n", response.Body.String())
}

func TestUploadFileRejectsUnsafeFileName(t *testing.T) {
	orgID := uuid.New()
	fileService := &stubFileService{
		uploadFileFn: func(context.Context, uuid.UUID, int, string, io.Reader) error {
			return service.ErrInvalidFileName
		},
	}

	response := performFileRequest(t, fileService, http.MethodPost, "/upload", orgID.String(), `bad\name.txt`)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Equal(t, "invalid file name\n", response.Body.String())
}
