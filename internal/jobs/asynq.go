package jobs

import "github.com/hibiken/asynq"

// Client хранит клиент Asynq и связанные настройки очередей.
type Client struct {
	client      *asynq.Client
	concurrency int
}

// New создаёт клиент Asynq для отправки задач в Redis.
func New(redisOpts asynq.RedisConnOpt, concurrency int) *Client {
	return &Client{
		client:      asynq.NewClient(redisOpts),
		concurrency: concurrency,
	}
}

// Close закрывает клиент Asynq.
func (c *Client) Close() error {
	if c == nil || c.client == nil {
		return nil
	}

	return c.client.Close()
}

// Concurrency возвращает настроенный уровень параллелизма очередей.
func (c *Client) Concurrency() int {
	return c.concurrency
}
