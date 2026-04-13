package auth

import "errors"

var (
	// ErrEmailAlreadyExists возвращается, когда email уже занят другим пользователем.
	ErrEmailAlreadyExists = errors.New("email already exists")

	// ErrInvalidCredentials возвращается при неверной паре email и пароль.
	ErrInvalidCredentials = errors.New("invalid credentials")

	// ErrUnauthorized возвращается, когда токен сессии отсутствует, истёк или отозван.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrPasswordTooShort возвращается, когда пароль короче минимального лимита.
	ErrPasswordTooShort = errors.New("password too short")

	// ErrOAuthNotConfigured возвращается, когда frontend уже знает маршрут,
	// но провайдер ещё не настроен на backend.
	ErrOAuthNotConfigured = errors.New("oauth provider is not configured")

	// ErrInvalidOAuthState возвращается, когда state отсутствует, истёк или уже был использован.
	ErrInvalidOAuthState = errors.New("oauth state is invalid")

	// ErrOAuthEmailNotVerified возвращается, когда провайдер не подтвердил email пользователя.
	ErrOAuthEmailNotVerified = errors.New("oauth email is not verified")

	// ErrOAuthEmailMissing возвращается, когда провайдер не вернул email, нужный для локального пользователя.
	ErrOAuthEmailMissing = errors.New("oauth email is missing")

	// ErrOAuthTokenInvalid возвращается, когда access token от OAuth-провайдера неверен или уже истёк.
	ErrOAuthTokenInvalid = errors.New("oauth access token is invalid")

	// ErrTelegramAuthNotConfigured возвращается, когда backend ещё не получил токен Telegram-бота.
	ErrTelegramAuthNotConfigured = errors.New("telegram auth is not configured")

	// ErrTelegramAuthInvalid возвращается, когда подпись Telegram не сошлась или обязательные поля отсутствуют.
	ErrTelegramAuthInvalid = errors.New("telegram auth data is invalid")

	// ErrTelegramAuthExpired возвращается, когда Telegram прислал слишком старые данные входа.
	ErrTelegramAuthExpired = errors.New("telegram auth data is expired")

	// ErrPasswordResetTokenInvalid возвращается, когда токен сброса не найден, истёк или уже использован.
	ErrPasswordResetTokenInvalid = errors.New("password reset token is invalid or expired")

	// ErrUserNotFound возвращается во внутренней логике, когда пользователь внезапно исчез из БД.
	ErrUserNotFound = errors.New("user not found")
)
