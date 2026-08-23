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
	"github.com/CORTA-11/core-api/internal/tenancy"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubFileService struct {
	uploadFileFn   func(context.Context, tenancy.TeamContext, string, io.Reader) error
	downloadFileFn func(context.Context, tenancy.TeamContext, string, io.Writer) error
}

func (stub *stubFileService) UploadFile(ctx context.Context, team tenancy.TeamContext, name string, input io.Reader) error {
	return stub.uploadFileFn(ctx, team, name, input)
}

func (stub *stubFileService) DownloadFile(ctx context.Context, team tenancy.TeamContext, name string, output io.Writer) error {
	return stub.downloadFileFn(ctx, team, name, output)
}

func performTrustedFileRequest(t *testing.T, fileService service.FileService, method, target, fileName string, includeJWT bool) *httptest.ResponseRecorder {
	t.Helper()
	teamID := uuid.MustParse("e9bffdf8-9c5b-4d2c-9923-ffda65a77402")
	tokenService := service.NewTokenService("file-handler-test-secret")
	token, err := tokenService.GenerateToken(uuid.New(), "file@example.test")
	require.NoError(t, err)
	router := chi.NewRouter()
	router.Mount("/{team}/files", fileRouter(&Router{
		fileService: fileService, tokenService: tokenService,
		tenantResolver: stubTeamResolver{resolveTeamFn: func(_ context.Context, _ tenancy.OrganizationContext, got uuid.UUID) (tenancy.TeamContext, error) {
			assert.Equal(t, teamID, got)
			return tenancy.TeamContext{}, nil
		}},
	}))

	requestTarget := "/" + teamID.String() + "/files" + target
	request := httptest.NewRequest(method, requestTarget, nil)
	if fileName != "" {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, createErr := writer.CreateFormFile("file", fileName)
		require.NoError(t, createErr)
		_, createErr = part.Write([]byte("file contents"))
		require.NoError(t, createErr)
		require.NoError(t, writer.Close())
		request = httptest.NewRequest(method, requestTarget, &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
	}
	request.Header.Set("X-Org-ID", "5dc89da9-f554-43ea-970b-d1ca15e2f921")
	if includeJWT {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestFileRoutesUseTrustedTeamContext(t *testing.T) {
	upload := &stubFileService{uploadFileFn: func(_ context.Context, _ tenancy.TeamContext, name string, input io.Reader) error {
		assert.Equal(t, "report.pdf", name)
		contents, err := io.ReadAll(input)
		require.NoError(t, err)
		assert.Equal(t, "file contents", string(contents))
		return nil
	}}
	response := performTrustedFileRequest(t, upload, http.MethodPost, "/upload", "report.pdf", true)
	assert.Equal(t, http.StatusOK, response.Code)

	download := &stubFileService{downloadFileFn: func(_ context.Context, _ tenancy.TeamContext, name string, output io.Writer) error {
		assert.Equal(t, "report.pdf", name)
		_, err := output.Write([]byte("downloaded contents"))
		return err
	}}
	response = performTrustedFileRequest(t, download, http.MethodGet, "/download/report.pdf", "", true)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "downloaded contents", response.Body.String())
	assert.Equal(t, `attachment; filename=report.pdf`, response.Header().Get("Content-Disposition"))
}

func TestFileRoutesRequireJWTAndUUIDTeam(t *testing.T) {
	response := performTrustedFileRequest(t, &stubFileService{}, http.MethodGet, "/download/report.pdf", "", false)
	assert.Equal(t, http.StatusUnauthorized, response.Code)

	tokenService := service.NewTokenService("file-handler-test-secret")
	token, err := tokenService.GenerateToken(uuid.New(), "file@example.test")
	require.NoError(t, err)
	router := chi.NewRouter()
	router.Mount("/{team}/files", fileRouter(&Router{fileService: &stubFileService{}, tokenService: tokenService, tenantResolver: stubTeamResolver{}}))
	request := httptest.NewRequest(http.MethodGet, "/not-a-uuid/files/download/report.pdf", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Org-ID", uuid.NewString())
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestUploadFileRejectsUnsafeName(t *testing.T) {
	fileService := &stubFileService{uploadFileFn: func(context.Context, tenancy.TeamContext, string, io.Reader) error {
		return service.ErrInvalidFileName
	}}
	response := performTrustedFileRequest(t, fileService, http.MethodPost, "/upload", `bad\name.txt`, true)
	assert.Equal(t, http.StatusBadRequest, response.Code)
}
