package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Client хранит пул подключений к PostgreSQL.
type Client struct {
	Pool *pgxpool.Pool
}

// New создаёт пул подключений и сразу проверяет доступность PostgreSQL.
//
// Быстрая проверка на старте нужна, чтобы приложение не запускалось в
// "полуживом" состоянии, когда процесс уже поднялся, а база ещё недоступна.
func New(dsn string) (*Client, error) {
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &Client{Pool: pool}, nil
}

// Ping проверяет, что пул подключений к PostgreSQL доступен.
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.Pool == nil {
		return nil
	}

	return c.Pool.Ping(ctx)
}

// Close закрывает пул подключений к PostgreSQL.
func (c *Client) Close() {
	if c == nil || c.Pool == nil {
		return
	}

	c.Pool.Close()
}
