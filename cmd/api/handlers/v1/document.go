package v1

import (
	"net/http"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/service"
)

type documentRequest struct {
	Title string `json:"title"`
}

type documentPatchRequest struct {
	Title    *string `json:"title"`
	BodyHTML *string `json:"body_html"`
}

func (handler *ResourceHandler) listDocuments(writer http.ResponseWriter, request *http.Request) {
	authentication, organizationID, teamID, ok := handler.scoped(request, true)
	if !ok || handler.documents == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	documents, err := handler.documents.List(request.Context(), authentication.Principal, organizationID, teamID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, struct {
		Items []service.DocumentView `json:"items"`
	}{Items: documents})
}

func (handler *ResourceHandler) createDocument(writer http.ResponseWriter, request *http.Request) {
	authentication, organizationID, teamID, ok := handler.scoped(request, true)
	var input documentRequest
	err := httpx.DecodeJSON(request, &input, maximumResourceBodyBytes)
	if !ok || handler.documents == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err != nil {
		_ = httpx.WriteProblem(writer, request, httpx.DecodeProblem(err))
		return
	}
	document, err := handler.documents.Create(request.Context(), authentication.Principal, organizationID, teamID, input.Title)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusCreated, document)
}

func (handler *ResourceHandler) getDocument(writer http.ResponseWriter, request *http.Request) {
	authentication, organizationID, teamID, ok := handler.scoped(request, true)
	documentID, valid := routeUUID(request, "document_id")
	if !ok || !valid || handler.documents == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	document, err := handler.documents.Get(request.Context(), authentication.Principal, organizationID, teamID, documentID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, document)
}

func (handler *ResourceHandler) updateDocument(writer http.ResponseWriter, request *http.Request) {
	authentication, organizationID, teamID, ok := handler.scoped(request, true)
	documentID, valid := routeUUID(request, "document_id")
	var input documentPatchRequest
	err := httpx.DecodeJSON(request, &input, maximumResourceBodyBytes)
	if !ok || !valid || handler.documents == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err != nil {
		_ = httpx.WriteProblem(writer, request, httpx.DecodeProblem(err))
		return
	}
	document, err := handler.documents.Update(request.Context(), authentication.Principal, organizationID, teamID,
		documentID, service.DocumentPatch{Title: input.Title, BodyHTML: input.BodyHTML})
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, document)
}

func (handler *ResourceHandler) deleteDocument(writer http.ResponseWriter, request *http.Request) {
	authentication, organizationID, teamID, ok := handler.scoped(request, true)
	documentID, valid := routeUUID(request, "document_id")
	if !ok || !valid || handler.documents == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err := handler.documents.Delete(request.Context(), authentication.Principal, organizationID, teamID, documentID); err != nil {
		handler.problem(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (handler *ResourceHandler) issueDocumentSocketTicket(writer http.ResponseWriter, request *http.Request) {
	authentication, organizationID, teamID, ok := handler.scoped(request, true)
	documentID, valid := routeUUID(request, "document_id")
	if !ok || !valid || handler.documents == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	ticket, err := handler.documents.IssueSocketTicket(
		request.Context(), authentication.Principal, authentication.User.DisplayName, organizationID, teamID, documentID,
	)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, struct {
		Token string `json:"token"`
	}{Token: ticket})
}
