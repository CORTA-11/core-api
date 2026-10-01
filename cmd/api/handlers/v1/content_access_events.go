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

// Database snapshots make reconnects and multi-replica deployments reliable.
// Every poll rechecks membership; the short lifetime also revalidates sessions.
func (h *ResourceHandler) contentAccessEvents(w http.ResponseWriter, r *http.Request) {
	auth, org, team, ok := h.scoped(r, true)
	if !ok || h.contentAccess == nil {
		h.problem(w, r, authorization.ErrResourceNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	controller := http.NewResponseController(w)
	var previous []byte
	started := false
	for {
		snapshot, err := h.contentAccess.List(ctx, auth.Principal, org, team)
		if err != nil {
			if !started {
				h.problem(w, r, err)
			}
			return
		}
		current, err := json.Marshal(snapshot)
		if err != nil {
			return
		}
		if !started {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache, no-transform")
			w.Header().Set("X-Accel-Buffering", "no")
			started = true
		}
		if !bytes.Equal(previous, current) {
			if _, err := fmt.Fprintf(w, "event: content-access\ndata: %s\n\n", current); err != nil {
				return
			}
			previous = current
		} else if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
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
