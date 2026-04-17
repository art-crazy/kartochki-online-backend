// Пакет email содержит адаптеры для отправки писем пользователям.
package email

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"kartochki-online-backend/internal/auth"
)

// NoopSender — заглушка отправителя, которая логирует письмо вместо реальной отправки.
//
// Используется для локальной разработки и окружений без SMTP.
// Токен выводится в лог, чтобы разработчик мог его использовать вручную.
type NoopSender struct {
	logger zerolog.Logger
}

// NewNoopSender создаёт заглушку-отправитель с логированием в zerolog.
func NewNoopSender(logger zerolog.Logger) *NoopSender {
	return &NoopSender{logger: logger}
}

// SendPasswordResetEmail логирует письмо сброса пароля вместо его реальной отправки.
//
// ВНИМАНИЕ: сырой токен выводится в лог намеренно — только для локальной разработки.
// Реальная реализация EmailSender не должна логировать токен ни при каких условиях.
func (s *NoopSender) SendPasswordResetEmail(_ context.Context, toEmail string, token string) error {
	s.logger.Info().
		Str("to", toEmail).
		Str("reset_token", token).
		Msg("noop email sender: password reset email (not sent)")
	return nil
}

// SendRegistrationVerificationEmail логирует код подтверждения регистрации вместо реальной отправки.
func (s *NoopSender) SendRegistrationVerificationEmail(_ context.Context, toEmail string, code string, expiresIn time.Duration) error {
	s.logger.Info().
		Str("to", toEmail).
		Str("verification_code", code).
		Dur("expires_in", expiresIn).
		Msg("noop email sender: registration verification email (not sent)")
	return nil
}

// Проверка на этапе компиляции: NoopSender должен реализовывать auth.EmailSender.
// Если интерфейс изменится, компилятор сразу укажет на расхождение.
var _ auth.EmailSender = (*NoopSender)(nil)
