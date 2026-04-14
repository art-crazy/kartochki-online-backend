package http

import (
	"errors"
	stdhttp "net/http"
	"strings"

	"kartochki-online-backend/internal/auth"
	"kartochki-online-backend/internal/http/authctx"
	"kartochki-online-backend/internal/http/response"
)

// authMiddleware проверяет токен сессии (из Bearer-заголовка или куки) и кладёт пользователя в context.
type authMiddleware struct {
	authService *auth.Service
}

// newAuthMiddleware создаёт middleware для защищённых endpoint.
func newAuthMiddleware(authService *auth.Service) authMiddleware {
	return authMiddleware{authService: authService}
}

// RequireUser защищает endpoint: извлекает токен из Bearer-заголовка или куки, проверяет сессию и кладёт пользователя в context.
func (m authMiddleware) RequireUser(next stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		token, err := tokenFromRequest(r)
		if err != nil {
			response.WriteError(w, r, stdhttp.StatusUnauthorized, "unauthorized", "authorization token is required")
			return
		}

		user, err := m.authService.Authenticate(r.Context(), token)
		if err != nil {
			if errors.Is(err, auth.ErrUnauthorized) {
				response.WriteError(w, r, stdhttp.StatusUnauthorized, "unauthorized", "authorization token is invalid")
				return
			}

			response.WriteError(w, r, stdhttp.StatusInternalServerError, "internal_error", "failed to authenticate request")
			return
		}

		ctx := authctx.WithUser(r.Context(), user)
		ctx = authctx.WithAccessToken(ctx, token)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// tokenFromRequest извлекает токен из Authorization: Bearer, при отсутствии — из куки auth_token.
func tokenFromRequest(r *stdhttp.Request) (string, error) {
	// Приоритет: явный Bearer-заголовок (нужен для API-клиентов и периода миграции).
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header != "" {
		scheme, token, ok := strings.Cut(header, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") {
			return "", auth.ErrUnauthorized
		}
		token = strings.TrimSpace(token)
		if token == "" {
			return "", auth.ErrUnauthorized
		}
		return token, nil
	}

	// Fallback: кука — браузер прикладывает её автоматически.
	cookie, err := r.Cookie("auth_token")
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", auth.ErrUnauthorized
	}

	return cookie.Value, nil
}
