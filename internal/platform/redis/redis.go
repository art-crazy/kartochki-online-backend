package redis

import (
	"context"

	"github.com/hibiken/asynq"
	redisv9 "github.com/redis/go-redis/v9"
)

type Client struct {
	inner *redisv9.Client
}

func New(addr string, password string, db int) (*Client, error) {
	client := redisv9.NewClient(&redisv9.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	return &Client{inner: client}, nil
}

func (c *Client) Close() error {
	if c == nil || c.inner == nil {
		return nil
	}

	return c.inner.Close()
}

func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.inner == nil {
		return nil
	}

	return c.inner.Ping(ctx).Err()
}

func (c *Client) AsynqOpt() asynq.RedisConnOpt {
	return asynq.RedisClientOpt{
		Addr:     c.inner.Options().Addr,
		Username: c.inner.Options().Username,
		Password: c.inner.Options().Password,
		DB:       c.inner.Options().DB,
	}
}
