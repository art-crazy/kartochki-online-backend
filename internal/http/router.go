package http

import (
	stdhttp "net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"kartochki-online-backend/internal/config"
	"kartochki-online-backend/internal/http/handlers"
	"kartochki-online-backend/internal/http/requestctx"
	"kartochki-online-backend/internal/http/response"
)

// NewRouter собирает HTTP-маршруты и middleware для публичного API и служебных endpoint.
func NewRouter(cfg config.HTTPConfig, logger zerolog.Logger, healthHandler handlers.HealthHandler) stdhttp.Handler {
	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(cfg.RequestTimeout))
	router.Use(requestctx.WithLogger(logger))
	router.Use(requestLogger(logger))

	// Единый fallback нужен заранее, чтобы фронтенд не зависел от разных форматов ошибок.
	router.NotFound(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		response.WriteError(w, r, stdhttp.StatusNotFound, "not_found", "resource not found")
	})

	router.MethodNotAllowed(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		response.WriteError(
			w,
			r,
			stdhttp.StatusMethodNotAllowed,
			"method_not_allowed",
			"method is not allowed for this resource",
		)
	})

	router.Get("/health/live", healthHandler.Live)
	router.Get("/health/ready", healthHandler.Ready)

	registerDocsRoutes(router)

	return router
}
