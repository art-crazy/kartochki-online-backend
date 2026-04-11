// Package requestctx хранит request-scoped данные transport-слоя.
package requestctx

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
)

type loggerContextKey struct{}

// WithLogger добавляет в context logger, уже обогащённый полями текущего запроса.
func WithLogger(baseLogger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestLogger := baseLogger.With().
				Str("request_id", middleware.GetReqID(r.Context())).
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Logger()

			ctx := context.WithValue(r.Context(), loggerContextKey{}, requestLogger)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Logger возвращает logger из context или fallback, если middleware ещё не отработал.
func Logger(ctx context.Context, fallback zerolog.Logger) zerolog.Logger {
	logger, ok := ctx.Value(loggerContextKey{}).(zerolog.Logger)
	if !ok {
		return fallback
	}

	return logger
}
