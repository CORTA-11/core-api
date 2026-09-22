package v1

import (
	"net/http"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/service"
)

type registerDeviceRequest struct {
	Token    string `json:"token"`
	Platform string `json:"platform"`
}

func (handler *ResourceHandler) registerDevice(writer http.ResponseWriter, request *http.Request) {
	authentication, ok := authenticationFrom(request)
	if !ok || handler.devices == nil {
		handler.problem(writer, request, authorization.ErrUnauthenticated)
		return
	}
	var input registerDeviceRequest
	if err := httpx.DecodeJSON(request, &input, maximumResourceBodyBytes); err != nil {
		_ = httpx.WriteProblem(writer, request, httpx.DecodeProblem(err))
		return
	}
	if input.Token == "" {
		handler.problem(writer, request, service.ErrInvalidInput)
		return
	}
	if err := handler.devices.RegisterDevice(request.Context(), authentication.Principal.UserID, input.Token, input.Platform); err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}
