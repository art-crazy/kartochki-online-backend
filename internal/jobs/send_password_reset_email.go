package jobs

import (
	"context"

	"github.com/hibiken/asynq"
)

// SendPasswordResetEmailPayload оставлен как совместимый alias к универсальному payload.
type SendPasswordResetEmailPayload = SendAuthEmailPayload

// EnqueueSendPasswordResetEmail оставлен как совместимая обёртка над единым механизмом auth-писем.
func (c *Client) EnqueueSendPasswordResetEmail(ctx context.Context, payload SendPasswordResetEmailPayload, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	payload.Kind = AuthEmailKindPasswordReset
	return c.EnqueueSendAuthEmail(ctx, payload, opts...)
}
