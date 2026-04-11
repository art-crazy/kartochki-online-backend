package contracts

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

// ForgotPasswordRequest описывает запрос на отправку ссылки для сброса пароля.
type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

// AuthResponse возвращается после успешной авторизации или регистрации.
type AuthResponse struct {
	User AuthUser `json:"user"`
}

// AuthUser содержит минимальные данные пользователя для auth-flow.
type AuthUser struct {
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

// ForgotPasswordResponse подтверждает, что запрос принят в обработку.
//
// Status стоит держать фиксированным, например `accepted`, чтобы не плодить
// лишние состояния.
type ForgotPasswordResponse struct {
	Status string `json:"status"`
}
