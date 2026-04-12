package jobs

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/hibiken/asynq"
)

const taskTypeGeneration = "generation:run"

// GenerationPayload описывает задачу фоновой генерации карточек.
// Worker получает только generation_id и сам перечитывает актуальное состояние из БД.
// Одна generation-задача должна существовать в очереди в единственном экземпляре.
type GenerationPayload struct {
	GenerationID string `json:"generation_id"`
}

// EnqueueGeneration ставит задачу генерации карточек в очередь Asynq.
func (c *Client) EnqueueGeneration(ctx context.Context, payload GenerationPayload, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("asynq client is not configured")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	// Повторы отключаем явно: текущая обработка пишет файлы и БД поэтапно,
	// поэтому безопаснее показать статус failed и дать frontend запустить генерацию заново.
	options := append([]asynq.Option{
		asynq.MaxRetry(0),
		// TaskID не даёт случайно поставить одну и ту же generation-задачу в очередь повторно.
		asynq.TaskID(payload.GenerationID),
	}, opts...)
	task := asynq.NewTask(taskTypeGeneration, body, options...)
	return c.client.EnqueueContext(ctx, task)
}
