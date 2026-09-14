package v1

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/CORTA-11/core-api/internal/pagination"
	"github.com/google/uuid"
)

type taskRequest struct {
	Description string     `json:"description"`
	Status      string     `json:"status"`
	AssigneeID  *uuid.UUID `json:"assignee_id"`
	// assigneeSet separates an omitted field (keep current) from an explicit
	// null (unassign); encoding/json cannot express that on a plain pointer.
	assigneeSet bool
}

// UnmarshalJSON decodes JSON into the value.
func (task *taskRequest) UnmarshalJSON(data []byte) error {
	var raw struct {
		Description string          `json:"description"`
		Status      string          `json:"status"`
		AssigneeID  json.RawMessage `json:"assignee_id"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	task.Description = raw.Description
	task.Status = raw.Status
	if raw.AssigneeID == nil {
		return nil
	}
	task.assigneeSet = true
	if bytes.Equal(bytes.TrimSpace(raw.AssigneeID), []byte("null")) {
		return nil
	}
	var value uuid.UUID
	if err := json.Unmarshal(raw.AssigneeID, &value); err != nil {
		return err
	}
	task.AssigneeID = &value
	return nil
}

func decodeTask(request *http.Request) (taskRequest, error) {
	var input taskRequest
	err := httpx.DecodeJSON(request, &input, maximumResourceBodyBytes)
	return input, err
}

func (handler *ResourceHandler) listTasks(writer http.ResponseWriter, request *http.Request) {
	authentication, organizationID, teamID, ok := handler.scoped(request, true)
	parameters, err := pagination.Parse(request.URL.Query())
	if !ok || err != nil || handler.teamTasks == nil {
		handler.problem(writer, request, firstError(err, authorization.ErrResourceNotFound))
		return
	}
	page, err := handler.teamTasks.ListTasks(request.Context(), authentication.Principal, organizationID, teamID, parameters)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, page)
}

func (handler *ResourceHandler) createTask(writer http.ResponseWriter, request *http.Request) {
	authentication, organizationID, teamID, ok := handler.scoped(request, true)
	input, err := decodeTask(request)
	if !ok || handler.teamTasks == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err != nil {
		_ = httpx.WriteProblem(writer, request, httpx.DecodeProblem(err))
		return
	}
	task, err := handler.teamTasks.CreateTask(request.Context(), authentication.Principal,
		organizationID, teamID, input.Description, input.Status, input.AssigneeID)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusCreated, task)
}

func (handler *ResourceHandler) updateTask(writer http.ResponseWriter, request *http.Request) {
	authentication, organizationID, teamID, ok := handler.scoped(request, true)
	taskID, validTask := routeUUID(request, "task_id")
	input, err := decodeTask(request)
	if !ok || !validTask || handler.teamTasks == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err != nil {
		_ = httpx.WriteProblem(writer, request, httpx.DecodeProblem(err))
		return
	}
	task, err := handler.teamTasks.UpdateTask(request.Context(), authentication.Principal,
		organizationID, teamID, taskID, input.Description, input.Status, input.AssigneeID, input.assigneeSet)
	if err != nil {
		handler.problem(writer, request, err)
		return
	}
	_ = httpx.WriteJSON(writer, http.StatusOK, task)
}

func (handler *ResourceHandler) deleteTask(writer http.ResponseWriter, request *http.Request) {
	authentication, organizationID, teamID, ok := handler.scoped(request, true)
	taskID, validTask := routeUUID(request, "task_id")
	if !ok || !validTask || handler.teamTasks == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	if err := handler.teamTasks.DeleteTask(request.Context(), authentication.Principal,
		organizationID, teamID, taskID); err != nil {
		handler.problem(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}
