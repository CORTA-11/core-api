package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/CORTA-11/core-api/internal/authorization"
)

// Snapshots survive reconnects and work across API replicas without a local bus.
func (handler *ResourceHandler) keyAccessEvents(writer http.ResponseWriter, request *http.Request) {
	authentication, ok := authenticationFrom(request)
	orgID, validOrg := routeUUID(request, "org_id")
	teamID, validTeam := routeUUID(request, "team_id")
	if !ok || !validOrg || !validTeam || handler.keyAccess == nil {
		handler.problem(writer, request, authorization.ErrResourceNotFound)
		return
	}
	// Reconnects revalidate the session; every snapshot rechecks membership.
	ctx, cancel := context.WithTimeout(request.Context(), 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	controller := http.NewResponseController(writer)
	var previous []byte
	started := false
	for {
		views, err := handler.keyAccess.ListKeyAccessRequests(ctx, authentication.Principal, orgID, teamID)
		if err != nil {
			if !started {
				handler.problem(writer, request, err)
			}
			return
		}
		current, err := json.Marshal(views)
		if err != nil {
			return
		}
		if !started {
			writer.Header().Set("Content-Type", "text/event-stream")
			writer.Header().Set("Cache-Control", "no-cache, no-transform")
			writer.Header().Set("X-Accel-Buffering", "no")
			started = true
		}
		if !bytes.Equal(previous, current) {
			if _, err := fmt.Fprintf(writer, "event: key-access-requests\ndata: %s\n\n", current); err != nil {
				return
			}
			previous = current
		} else if _, err := fmt.Fprint(writer, ": heartbeat\n\n"); err != nil {
			return
		}
		if err := controller.Flush(); err != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
