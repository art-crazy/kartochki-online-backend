package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	redisv9 "github.com/redis/go-redis/v9"
)

// Client хранит Redis-клиент для runtime-проверок и инфраструктурных зависимостей.
type Client struct {
	inner *redisv9.Client
}

// New создаёт Redis-клиент и сразу проверяет, что Redis доступен.
func New(addr string, password string, db int) (*Client, error) {
	client := redisv9.NewClient(&redisv9.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return &Client{inner: client}, nil
}

// Close закрывает Redis-клиент.
func (c *Client) Close() error {
	if c == nil || c.inner == nil {
		return nil
	}

	return c.inner.Close()
}

// Ping проверяет доступность Redis.
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.inner == nil {
		return nil
	}

	return c.inner.Ping(ctx).Err()
}

// AsynqOpt возвращает настройки подключения, которые использует Asynq.
func (c *Client) AsynqOpt() asynq.RedisConnOpt {
	return asynq.RedisClientOpt{
		Addr:     c.inner.Options().Addr,
		Username: c.inner.Options().Username,
		Password: c.inner.Options().Password,
		DB:       c.inner.Options().DB,
	}
}
