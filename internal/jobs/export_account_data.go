package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hibiken/asynq"
)

const taskTypeExportAccountData = "settings:export-account-data"

// ExportAccountDataPayload описывает задачу экспорта пользовательских данных.
// Payload делаем явным, чтобы будущий worker не зависел от неструктурированных map-полей.
type ExportAccountDataPayload struct {
	UserID      string    `json:"user_id"`
	UserEmail   string    `json:"user_email,omitempty"`
	RequestedAt time.Time `json:"requested_at"`
}

// EnqueueExportAccountData ставит экспорт данных аккаунта в очередь Asynq.
func (c *Client) EnqueueExportAccountData(ctx context.Context, payload ExportAccountDataPayload, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("asynq client is not configured")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	task := asynq.NewTask(taskTypeExportAccountData, body, opts...)
	return c.client.EnqueueContext(ctx, task)
}
