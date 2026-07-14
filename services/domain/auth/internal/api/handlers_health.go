package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type pinger interface {
	Ping(ctx context.Context) error
}

// Liveness handles GET /health/live — always returns 200 if the process is running.
func (h *Handlers) Liveness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Readiness handles GET /health/ready — checks downstream dependencies.
func (h *Handlers) Readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	checks := map[string]string{}
	allOK := true

	if err := h.db.Ping(ctx); err != nil {
		checks["postgres"] = err.Error()
		allOK = false
	} else {
		checks["postgres"] = "ok"
	}

	w.Header().Set("Content-Type", "application/json")
	if !allOK {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"status": checks})
}
