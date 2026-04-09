package app

import (
	"context"
	"errors"

	"github.com/rs/zerolog"

	"kartochki-online-backend/internal/config"
	"kartochki-online-backend/internal/health"
	httptransport "kartochki-online-backend/internal/http"
	"kartochki-online-backend/internal/http/handlers"
	"kartochki-online-backend/internal/httpserver"
	"kartochki-online-backend/internal/jobs"
	"kartochki-online-backend/internal/platform/postgres"
	"kartochki-online-backend/internal/platform/redis"
)

type App struct {
	Server *httpserver.Server
	DB     *postgres.Client
	Redis  *redis.Client
	Asynq  *jobs.Client
}

func New(cfg config.Config, logger zerolog.Logger) (*App, error) {
	db, err := postgres.New(cfg.Postgres.DSN)
	if err != nil {
		return nil, err
	}

	redisClient, err := redis.New(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		db.Close()
		return nil, err
	}

	asynqClient := jobs.New(redisClient.AsynqOpt(), cfg.Asynq.Concurrency)
	readiness := health.NewService(
		health.NewChecker("postgres", db.Ping),
		health.NewChecker("redis", redisClient.Ping),
	)
	healthHandler := handlers.NewHealthHandler(readiness)

	router := httptransport.NewRouter(cfg.HTTP, logger, healthHandler)
	server := httpserver.New(cfg.HTTP, router)

	return &App{
		Server: server,
		DB:     db,
		Redis:  redisClient,
		Asynq:  asynqClient,
	}, nil
}

func (a *App) Shutdown(ctx context.Context) error {
	var joined error

	if a.Server != nil {
		if err := a.Server.Shutdown(ctx); err != nil {
			joined = errors.Join(joined, err)
		}
	}

	if a.Asynq != nil {
		if err := a.Asynq.Close(); err != nil {
			joined = errors.Join(joined, err)
		}
	}

	if a.Redis != nil {
		if err := a.Redis.Close(); err != nil {
			joined = errors.Join(joined, err)
		}
	}

	if a.DB != nil {
		a.DB.Close()
	}

	return joined
}
