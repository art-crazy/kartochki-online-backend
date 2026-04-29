package jobs

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/hibiken/asynq"
)

const (
	taskTypeBillingRenewSubscriptions = "billing:renew-subscriptions"
	billingRenewalBatchLimit          = 50
)

// BillingRenewSubscriptionsPayload описывает периодическую задачу автопродления подписок.
type BillingRenewSubscriptionsPayload struct {
	BatchLimit int `json:"batch_limit"`
}

// EnqueueBillingRenewSubscriptions ставит задачу автопродления подписок в очередь.
func (c *Client) EnqueueBillingRenewSubscriptions(ctx context.Context, payload BillingRenewSubscriptionsPayload, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("asynq client is not configured")
	}

	if payload.BatchLimit <= 0 {
		payload.BatchLimit = billingRenewalBatchLimit
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	task := asynq.NewTask(taskTypeBillingRenewSubscriptions, body, append([]asynq.Option{asynq.MaxRetry(5)}, opts...)...)
	return c.client.EnqueueContext(ctx, task)
}
