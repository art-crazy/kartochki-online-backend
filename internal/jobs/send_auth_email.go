package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hibiken/asynq"
)

const (
	taskTypeSendAuthEmail = "auth:send-email"
	// authEmailMaxRetry даёт worker-у больше шансов пережить краткий сетевой сбой до SMTP.
	authEmailMaxRetry = 5
)

const (
	// AuthEmailKindPasswordReset отправляет письмо со ссылкой для сброса пароля.
	AuthEmailKindPasswordReset = "password_reset"
	// AuthEmailKindRegistrationVerification отправляет письмо с кодом подтверждения регистрации.
	AuthEmailKindRegistrationVerification = "registration_verification"
)

// SendAuthEmailPayload описывает универсальную задачу отправки auth-письма.
type SendAuthEmailPayload struct {
	UserID         string        `json:"user_id"`
	Kind           string        `json:"kind"`
	Email          string        `json:"email"`
	RawToken       string        `json:"raw_token,omitempty"`
	VerificationID string        `json:"verification_id,omitempty"`
	Code           string        `json:"code,omitempty"`
	ExpiresIn      time.Duration `json:"expires_in,omitempty"`
}

// EnqueueSendAuthEmail ставит auth-письмо в очередь Asynq.
func (c *Client) EnqueueSendAuthEmail(ctx context.Context, payload SendAuthEmailPayload, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("asynq client is not configured")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	task := asynq.NewTask(taskTypeSendAuthEmail, body, append([]asynq.Option{asynq.MaxRetry(authEmailMaxRetry)}, opts...)...)
	return c.client.EnqueueContext(ctx, task)
}

// EnqueuePasswordResetEmail реализует auth.AuthEmailEnqueuer для письма сброса пароля.
func (c *Client) EnqueuePasswordResetEmail(ctx context.Context, userID, email, rawToken string) error {
	_, err := c.EnqueueSendAuthEmail(ctx, SendAuthEmailPayload{
		UserID:   userID,
		Kind:     AuthEmailKindPasswordReset,
		Email:    email,
		RawToken: rawToken,
	})
	return err
}

// EnqueueRegistrationVerificationEmail реализует auth.AuthEmailEnqueuer для письма подтверждения регистрации.
func (c *Client) EnqueueRegistrationVerificationEmail(ctx context.Context, email, verificationID, code string, expiresIn time.Duration) error {
	_, err := c.EnqueueSendAuthEmail(ctx, SendAuthEmailPayload{
		Kind:           AuthEmailKindRegistrationVerification,
		Email:          email,
		VerificationID: verificationID,
		Code:           code,
		ExpiresIn:      expiresIn,
	})
	return err
}
