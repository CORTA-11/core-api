package v1

import (
	"errors"
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
	if errors.Is(err, service.ErrAISettingsMissing) {
		_ = httpx.WriteProblem(writer, request, httpx.NewError(httpx.ProblemAISettingsRequired, err))
		return
	}
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, result)
}

func (handler *ResourceHandler) teamAISettings(writer http.ResponseWriter, request *http.Request) {
	authentication, orgID, teamID, ok := handler.scoped(request, true)
	if !ok || handler.ai == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	var result service.TeamAISettingsView
	var err error
	if request.Method == http.MethodGet {
		result, err = handler.ai.GetSettings(request.Context(), authentication.Principal, orgID, teamID)
	} else {
		var input service.TeamAISettingsInput
		if err = httpx.DecodeJSON(request, &input, httpx.BodyLimitBytes(apicontract.BodyJSON)); err != nil {
			_ = httpx.WriteProblem(writer, request, httpx.DecodeProblem(err))
			return
		}
		result, err = handler.ai.SaveSettings(request.Context(), authentication.Principal, orgID, teamID, input)
	}
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, result)
}
