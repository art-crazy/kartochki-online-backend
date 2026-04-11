package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/rs/zerolog"

	"kartochki-online-backend/internal/http/requestctx"
	"kartochki-online-backend/internal/http/response"
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
	logger    zerolog.Logger
}

// NewHealthHandler создаёт обработчик health endpoint.
func NewHealthHandler(readiness ReadinessChecker, logger zerolog.Logger) HealthHandler {
	return HealthHandler{
		readiness: readiness,
		logger:    logger,
	}
}

// Live отвечает, что HTTP-процесс запущен и может принимать запросы.
func (h HealthHandler) Live(w http.ResponseWriter, r *http.Request) {
	response.WriteJSON(w, r, http.StatusOK, healthResponse{Status: "ok"})
}

// Ready проверяет доступность обязательных зависимостей приложения.
func (h HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	if h.readiness == nil {
		response.WriteJSON(w, r, http.StatusOK, healthResponse{Status: "ok"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.readiness.Ready(ctx); err != nil {
		requestLogger := requestctx.Logger(r.Context(), h.logger)
		requestLogger.Warn().
			Err(err).
			Msg("readiness check failed")
		response.WriteJSON(w, r, http.StatusServiceUnavailable, healthResponse{Status: "degraded"})
		return
	}

	response.WriteJSON(w, r, http.StatusOK, healthResponse{Status: "ok"})
}
