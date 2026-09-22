package v1

import (
	"net/http"

	"github.com/CORTA-11/core-api/internal/apicontract"
	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/service"
)

func (handler *ResourceHandler) processAI(writer http.ResponseWriter, request *http.Request) {
	authentication, organizationID, teamID, ok := handler.scoped(request, true)
	var input service.AIProcessInput
	err := httpx.DecodeJSON(request, &input, httpx.BodyLimitBytes(apicontract.BodyAI))
	if !ok || handler.ai == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err != nil {
		_ = httpx.WriteProblem(writer, request, httpx.DecodeProblem(err))
		return
	}
	result, err := handler.ai.Process(request.Context(), authentication.Principal, organizationID, teamID, input)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, result)
}
