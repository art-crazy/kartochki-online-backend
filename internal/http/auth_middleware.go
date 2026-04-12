package http

import (
	"errors"
	stdhttp "net/http"
	"strings"

	"kartochki-online-backend/internal/auth"
	"kartochki-online-backend/internal/http/authctx"
	"kartochki-online-backend/internal/http/response"
)

// authMiddleware проверяет Bearer-токен и кладёт пользователя в context.
type authMiddleware struct {
	authService *auth.Service
}

// newAuthMiddleware создаёт middleware для защищённых endpoint.
func newAuthMiddleware(authService *auth.Service) authMiddleware {
	return authMiddleware{authService: authService}
}

// RequireUser защищает endpoint и останавливает запрос, если сессия невалидна.
func (m authMiddleware) RequireUser(next stdhttp.Handler) stdhttp.Handler {
	return stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		token, err := bearerTokenFromRequest(r)
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

func bearerTokenFromRequest(r *stdhttp.Request) (string, error) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return "", auth.ErrUnauthorized
	}

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
