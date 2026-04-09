package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type healthResponse struct {
	Status string `json:"status"`
}

type ReadinessChecker interface {
	Ready(context.Context) error
}

type HealthHandler struct {
	readiness ReadinessChecker
}

func NewHealthHandler(readiness ReadinessChecker) HealthHandler {
	return HealthHandler{readiness: readiness}
}

func (h HealthHandler) Live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

func (h HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	if h.readiness == nil {
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.readiness.Ready(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "degraded"})
		return
	}

	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
