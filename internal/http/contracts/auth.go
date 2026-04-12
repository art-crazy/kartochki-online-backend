package contracts

import "time"

// LoginRequest описывает вход пользователя по email и паролю.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RegisterRequest описывает регистрацию пользователя по email и паролю.
type RegisterRequest struct {
	Name     string `json:"name,omitempty"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// TelegramLoginRequest описывает подписанные поля, которые frontend получает от Telegram Login Widget.
//
// Frontend не должен придумывать или менять эти значения. Backend проверяет их подпись
// по токену бота и только после этого создаёт локальную сессию.
type TelegramLoginRequest struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
	PhotoURL  string `json:"photo_url,omitempty"`
	AuthDate  int64  `json:"auth_date"`
	Hash      string `json:"hash"`
}

// ForgotPasswordRequest описывает запрос на отправку ссылки для сброса пароля.
type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

// AuthResponse возвращается после успешной авторизации или регистрации.
type AuthResponse struct {
	User    AuthUser    `json:"user"`
	Session AuthSession `json:"session"`
}

// AuthUser содержит минимальные данные пользователя для auth-flow.
type AuthUser struct {
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

// AuthSession описывает токен текущей сессии.
//
// Фронтенд хранит access token у себя и затем отправляет его как Bearer-токен.
type AuthSession struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// CurrentUserResponse возвращает данные уже авторизованного пользователя.
type CurrentUserResponse struct {
	User AuthUser `json:"user"`
}

// LogoutResponse подтверждает, что текущая сессия завершена.
type LogoutResponse struct {
	Status string `json:"status"`
}

// ForgotPasswordResponse подтверждает, что запрос принят в обработку.
//
// Status стоит держать фиксированным, например `accepted`, чтобы не плодить
// лишние состояния.
type ForgotPasswordResponse struct {
	Status string `json:"status"`
}
