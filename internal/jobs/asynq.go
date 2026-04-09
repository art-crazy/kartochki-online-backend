package jobs

import (
	"github.com/hibiken/asynq"
)

type Client struct {
	client      *asynq.Client
	concurrency int
}

func New(redisOpts asynq.RedisConnOpt, concurrency int) *Client {
	return &Client{
		client:      asynq.NewClient(redisOpts),
		concurrency: concurrency,
	}
}

func (c *Client) Close() error {
	if c == nil || c.client == nil {
		return nil
	}

	return c.client.Close()
}

func (c *Client) Concurrency() int {
	return c.concurrency
}
