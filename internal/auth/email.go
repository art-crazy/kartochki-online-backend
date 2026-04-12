package auth

import "context"

// EmailSender отправляет письма пользователям.
//
// Конкретная реализация живёт снаружи пакета auth — это может быть SMTP-адаптер,
// провайдер transactional-почты или заглушка для тестов.
type EmailSender interface {
	// SendPasswordResetEmail отправляет письмо со ссылкой для сброса пароля.
	//
	// token — сырой токен, который нужно встроить в URL на frontend.
	SendPasswordResetEmail(ctx context.Context, toEmail string, token string) error
}

// PasswordResetEmailEnqueuer ставит задачу отправки письма сброса пароля в фоновую очередь.
//
// Через этот интерфейс auth-сервис не зависит от конкретного брокера задач.
type PasswordResetEmailEnqueuer interface {
	// EnqueuePasswordResetEmail ставит письмо в очередь и не ждёт отправки.
	// userID нужен для структурированного логирования в worker-е.
	EnqueuePasswordResetEmail(ctx context.Context, userID, email, rawToken string) error
}
