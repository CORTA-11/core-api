package v1

import (
	"io"
	"net/http"
	"strconv"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/service"
)

// getPublicKeysForTeam gets public keys for team.
func (handler *ResourceHandler) getPublicKeysForTeam(writer http.ResponseWriter, request *http.Request) {
	authentication, ok := authenticationFrom(request)
	orgID, validOrg := routeUUID(request, "org_id")
	teamID, validTeam := routeUUID(request, "team_id")
	if !ok || !validOrg || !validTeam || handler.keys == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}

	keys, err := handler.keys.GetPublicKeysForTeam(request.Context(), authentication.Principal, orgID, teamID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}

	_ = httpx.WriteJSON(writer, http.StatusOK, keys)
}

// upsertTeamSharedKeys upserts team shared keys.
func (handler *ResourceHandler) upsertTeamSharedKeys(writer http.ResponseWriter, request *http.Request) {
	authentication, ok := authenticationFrom(request)
	orgID, validOrg := routeUUID(request, "org_id")
	teamID, validTeam := routeUUID(request, "team_id")
	if !ok || !validOrg || !validTeam || handler.keys == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}

	var input []service.TeamSharedKey
	if err := httpx.DecodeJSON(request, &input, maximumResourceBodyBytes); err != nil {
		_ = httpx.WriteProblem(writer, request, httpx.DecodeProblem(err))
		return
	}

	err := handler.keys.UpsertTeamSharedKeys(request.Context(), authentication.Principal, orgID, teamID, input)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}

	writer.WriteHeader(http.StatusNoContent)
}

// listTeamSharedKeysForUser lists team shared keys for user.
func (handler *ResourceHandler) listTeamSharedKeysForUser(writer http.ResponseWriter, request *http.Request) {
	authentication, ok := authenticationFrom(request)
	orgID, validOrg := routeUUID(request, "org_id")
	teamID, validTeam := routeUUID(request, "team_id")
	if !ok || !validOrg || !validTeam || handler.keys == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}

	keys, err := handler.keys.ListTeamSharedKeysForUser(request.Context(), authentication.Principal, orgID, teamID, authentication.Principal.UserID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}

	_ = httpx.WriteJSON(writer, http.StatusOK, keys)
}

// uploadFile uploads file.
func (handler *ResourceHandler) uploadFile(writer http.ResponseWriter, request *http.Request) {
	authentication, ok := authenticationFrom(request)
	orgID, validOrg := routeUUID(request, "org_id")
	teamID, validTeam := routeUUID(request, "team_id")
	if !ok || !validOrg || !validTeam || handler.files == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}

	err := request.ParseMultipartForm(10 << 20) // 10 MB limit
	if err != nil {
		handler.problem(writer, request, service.ErrInvalidInput)
		return
	}

	name := request.FormValue("name")
	keyVersionStr := request.FormValue("key_version")
	ivStr := request.FormValue("iv")

	keyVersion, err := strconv.Atoi(keyVersionStr)
	if err != nil {
		handler.problem(writer, request, service.ErrInvalidInput)
		return
	}

	iv := []byte(ivStr)

	file, fileHeader, err := request.FormFile("file")
	if err != nil {
		handler.problem(writer, request, service.ErrInvalidInput)
		return
	}
	defer func() { _ = file.Close() }()

	metadata, err := handler.files.UploadFile(
		request.Context(),
		authentication.Principal,
		orgID,
		teamID,
		name,
		fileHeader.Header.Get("Content-Type"),
		file,
		fileHeader.Size,
		iv,
		int32(keyVersion),
	)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}

	_ = httpx.WriteJSON(writer, http.StatusCreated, metadata)
}

// listFiles lists files.
func (handler *ResourceHandler) listFiles(writer http.ResponseWriter, request *http.Request) {
	authentication, ok := authenticationFrom(request)
	orgID, validOrg := routeUUID(request, "org_id")
	teamID, validTeam := routeUUID(request, "team_id")
	if !ok || !validOrg || !validTeam || handler.files == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}

	files, err := handler.files.ListFiles(request.Context(), authentication.Principal, orgID, teamID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}

	_ = httpx.WriteJSON(writer, http.StatusOK, files)
}

// downloadFile downloads file.
func (handler *ResourceHandler) downloadFile(writer http.ResponseWriter, request *http.Request) {
	authentication, ok := authenticationFrom(request)
	orgID, validOrg := routeUUID(request, "org_id")
	teamID, validTeam := routeUUID(request, "team_id")
	fileID, validFile := routeUUID(request, "file_id")
	if !ok || !validOrg || !validTeam || !validFile || handler.files == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}

	metadata, stream, err := handler.files.DownloadFile(request.Context(), authentication.Principal, orgID, teamID, fileID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	defer func() { _ = stream.Close() }()

	// X-File-IV is deliberately absent: the seal header carries the IV inside
	// the encrypted bytes, and raw ciphertext bytes are not valid ASCII header
	// values — undici (Next's proxy) rejects them and the download 500s.
	writer.Header().Set("Content-Type", metadata.ContentType)
	writer.Header().Set("Content-Length", strconv.FormatInt(metadata.Size, 10))
	writer.Header().Set("X-File-Name", metadata.Name)
	writer.Header().Set("X-File-Key-Version", strconv.Itoa(int(metadata.KeyVersion)))

	_, _ = io.Copy(writer, stream)
}

// deleteFile deletes file.
func (handler *ResourceHandler) deleteFile(writer http.ResponseWriter, request *http.Request) {
	authentication, ok := authenticationFrom(request)
	orgID, validOrg := routeUUID(request, "org_id")
	teamID, validTeam := routeUUID(request, "team_id")
	fileID, validFile := routeUUID(request, "file_id")
	if !ok || !validOrg || !validTeam || !validFile || handler.files == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}

	err := handler.files.DeleteFile(request.Context(), authentication.Principal, orgID, teamID, fileID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}

	writer.WriteHeader(http.StatusNoContent)
}
