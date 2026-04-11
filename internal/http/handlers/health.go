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

// ReadinessChecker описывает минимальный контракт readiness-проверки.
type ReadinessChecker interface {
	Ready(context.Context) error
}

// HealthHandler обслуживает liveness и readiness endpoint приложения.
type HealthHandler struct {
	readiness ReadinessChecker
}

// NewHealthHandler создаёт обработчик health endpoint.
func NewHealthHandler(readiness ReadinessChecker) HealthHandler {
	return HealthHandler{readiness: readiness}
}

// Live отвечает, что HTTP-процесс запущен и может принимать запросы.
func (h HealthHandler) Live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

// Ready проверяет доступность обязательных зависимостей приложения.
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

// writeJSON пишет JSON-ответ без дополнительной бизнес-логики.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
