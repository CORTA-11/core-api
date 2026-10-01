package v1

import (
	"net/http"

	"github.com/CORTA-11/core-api/internal/authorization"
	"github.com/CORTA-11/core-api/internal/httpx"
	"github.com/google/uuid"
)

type contentAccessInput struct {
	Kind       string    `json:"kind"`
	ResourceID uuid.UUID `json:"resource_id"`
	MemberID   uuid.UUID `json:"member_id"`
}

func (h *ResourceHandler) listContentAccess(w http.ResponseWriter, r *http.Request) {
	auth, org, team, ok := h.scoped(r, true)
	if !ok || h.contentAccess == nil {
		h.problem(w, r, authorization.ErrResourceNotFound)
		return
	}
	snapshot, err := h.contentAccess.List(r.Context(), auth.Principal, org, team)
	if err != nil {
		h.problem(w, r, err)
		return
	}
	_ = httpx.WriteJSON(w, http.StatusOK, snapshot)
}

func (h *ResourceHandler) requestContentAccess(w http.ResponseWriter, r *http.Request) {
	h.writeContentAccess(w, r, false)
}
func (h *ResourceHandler) grantContentAccess(w http.ResponseWriter, r *http.Request) {
	h.writeContentAccess(w, r, true)
}

func (h *ResourceHandler) writeContentAccess(w http.ResponseWriter, r *http.Request, grant bool) {
	auth, org, team, ok := h.scoped(r, true)
	if !ok || h.contentAccess == nil {
		h.problem(w, r, authorization.ErrResourceNotFound)
		return
	}
	var input contentAccessInput
	if err := httpx.DecodeJSON(r, &input, maximumResourceBodyBytes); err != nil {
		_ = httpx.WriteProblem(w, r, httpx.DecodeProblem(err))
		return
	}
	var err error
	if grant {
		err = h.contentAccess.Grant(r.Context(), auth.Principal, org, team, input.Kind, input.ResourceID, input.MemberID)
	} else {
		err = h.contentAccess.Request(r.Context(), auth.Principal, org, team, input.Kind, input.ResourceID)
	}
	if err != nil {
		h.problem(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ResourceHandler) approveContentAccess(w http.ResponseWriter, r *http.Request) {
	h.decideContentAccess(w, r, "granted")
}
func (h *ResourceHandler) denyContentAccess(w http.ResponseWriter, r *http.Request) {
	h.decideContentAccess(w, r, "denied")
}

func (h *ResourceHandler) decideContentAccess(w http.ResponseWriter, r *http.Request, status string) {
	auth, org, team, ok := h.scoped(r, true)
	id, valid := routeUUID(r, "request_id")
	if !ok || !valid || h.contentAccess == nil {
		h.problem(w, r, authorization.ErrResourceNotFound)
		return
	}
	if err := h.contentAccess.Decide(r.Context(), auth.Principal, org, team, id, status); err != nil {
		h.problem(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
