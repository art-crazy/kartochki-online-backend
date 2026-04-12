package authctx

import (
	"context"

	"kartochki-online-backend/internal/auth"
)

type contextKey string

const (
	currentUserKey contextKey = "current_auth_user"
	accessTokenKey contextKey = "current_access_token"
)

// WithUser сохраняет текущего пользователя в context запроса.
func WithUser(ctx context.Context, user auth.User) context.Context {
	return context.WithValue(ctx, currentUserKey, user)
}

// WithAccessToken сохраняет текущий Bearer-токен в context.
func WithAccessToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, accessTokenKey, token)
}

// User достаёт текущего пользователя из context.
func User(ctx context.Context) (auth.User, bool) {
	user, ok := ctx.Value(currentUserKey).(auth.User)
	return user, ok
}

// AccessToken достаёт Bearer-токен текущей сессии из context.
func AccessToken(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(accessTokenKey).(string)
	return token, ok
}
