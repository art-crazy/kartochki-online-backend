package http

import (
	"io/fs"
	stdhttp "net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	openapi "kartochki-online-backend/api/openapi"
	"kartochki-online-backend/internal/config"
	"kartochki-online-backend/internal/http/handlers"
)

func NewRouter(cfg config.HTTPConfig, logger zerolog.Logger, healthHandler handlers.HealthHandler) stdhttp.Handler {
	router := chi.NewRouter()

	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(cfg.RequestTimeout))
	router.Use(requestLogger(logger))

	router.Get("/health/live", healthHandler.Live)
	router.Get("/health/ready", healthHandler.Ready)

	openapiFS, err := fs.Sub(openapi.Files, ".")
	if err == nil {
		router.Handle("/openapi/*", stdhttp.StripPrefix("/openapi/", stdhttp.FileServer(stdhttp.FS(openapiFS))))
	}

	return router
}
