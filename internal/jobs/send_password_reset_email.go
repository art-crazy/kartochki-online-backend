package jobs

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/hibiken/asynq"
)

const taskTypeSendPasswordResetEmail = "auth:send-password-reset-email"

// SendPasswordResetEmailPayload описывает задачу отправки письма для сброса пароля.
// Храним rawToken — тот же, что передаётся пользователю в URL reset-ссылки.
// Worker должен быть идемпотентен: повторная отправка того же письма безопасна,
// если токен ещё не истёк; просроченный токен просто не пройдёт валидацию при переходе по ссылке.
type SendPasswordResetEmailPayload struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	RawToken string `json:"raw_token"`
}

// EnqueueSendPasswordResetEmail ставит отправку письма сброса пароля в очередь Asynq.
// Задача выполняется вне HTTP-запроса, чтобы зависание SMTP не держало соединение клиента.
func (c *Client) EnqueueSendPasswordResetEmail(ctx context.Context, payload SendPasswordResetEmailPayload, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("asynq client is not configured")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	// Разрешаем до 3 попыток: SMTP может быть временно недоступен,
	// а повторная отправка безопасна, пока токен действителен.
	options := append([]asynq.Option{
		asynq.MaxRetry(3),
	}, opts...)
	task := asynq.NewTask(taskTypeSendPasswordResetEmail, body, options...)
	return c.client.EnqueueContext(ctx, task)
}

// EnqueuePasswordResetEmail реализует auth.PasswordResetEmailEnqueuer.
// Метод адаптирует сигнатуру интерфейса auth-пакета к внутреннему EnqueueSendPasswordResetEmail.
func (c *Client) EnqueuePasswordResetEmail(ctx context.Context, userID, email, rawToken string) error {
	_, err := c.EnqueueSendPasswordResetEmail(ctx, SendPasswordResetEmailPayload{
		UserID:   userID,
		Email:    email,
		RawToken: rawToken,
	})
	return err
}
