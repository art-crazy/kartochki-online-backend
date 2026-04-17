package auth

import (
	"context"
	"time"
)

// EmailSender отправляет письма пользователям.
//
// Конкретная реализация живёт снаружи пакета auth — это может быть SMTP-адаптер,
// провайдер transactional-почты или заглушка для тестов.
type EmailSender interface {
	// SendPasswordResetEmail отправляет письмо со ссылкой для сброса пароля.
	//
	// token — сырой токен, который нужно встроить в URL на frontend.
	SendPasswordResetEmail(ctx context.Context, toEmail string, token string) error

	// SendRegistrationVerificationEmail отправляет письмо с одноразовым кодом подтверждения регистрации.
	// code передаётся в сыром виде, потому что пользователь должен ввести его на втором шаге регистрации.
	// verificationID нужен для fallback-ссылки, которая может завершить flow без поиска локального состояния на frontend.
	SendRegistrationVerificationEmail(ctx context.Context, toEmail string, verificationID string, code string, expiresIn time.Duration) error
}

// AuthEmailEnqueuer ставит auth-письма в фоновую очередь.
//
// Через этот интерфейс auth-сервис не зависит от конкретного брокера задач.
type AuthEmailEnqueuer interface {
	// EnqueuePasswordResetEmail ставит письмо сброса пароля в очередь и не ждёт отправки.
	EnqueuePasswordResetEmail(ctx context.Context, userID, email, rawToken string) error

	// EnqueueRegistrationVerificationEmail ставит письмо с кодом подтверждения регистрации в очередь.
	// verificationID передаётся в payload, чтобы worker собрал рабочую ссылку подтверждения.
	EnqueueRegistrationVerificationEmail(ctx context.Context, email, verificationID, code string, expiresIn time.Duration) error
}
