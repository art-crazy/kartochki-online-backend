package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Client struct {
	Pool *pgxpool.Pool
}

func New(dsn string) (*Client, error) {
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil, err
	}

	return &Client{Pool: pool}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.Pool == nil {
		return nil
	}

	return c.Pool.Ping(ctx)
}

func (c *Client) Close() {
	if c == nil || c.Pool == nil {
		return
	}

	c.Pool.Close()
}
