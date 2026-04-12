package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog"

	"kartochki-online-backend/internal/auth"
	"kartochki-online-backend/internal/billing"
	"kartochki-online-backend/internal/blog"
	"kartochki-online-backend/internal/config"
	"kartochki-online-backend/internal/dbgen"
	"kartochki-online-backend/internal/generation"
	"kartochki-online-backend/internal/health"
	httptransport "kartochki-online-backend/internal/http"
	"kartochki-online-backend/internal/http/handlers"
	"kartochki-online-backend/internal/httpserver"
	"kartochki-online-backend/internal/jobs"
	"kartochki-online-backend/internal/platform/email"
	"kartochki-online-backend/internal/platform/postgres"
	"kartochki-online-backend/internal/platform/redis"
	"kartochki-online-backend/internal/platform/storage"
	"kartochki-online-backend/internal/projects"
	"kartochki-online-backend/internal/settings"
)

// generationBillingLimits адаптирует billing-сервис к минимальному контракту generation.
// Так generation не знает о деталях billing-домена и зависит только от проверяемого правила.
type generationBillingLimits struct {
	billing *billing.Service
}

// EnsureGenerationAllowed проверяет квоту и переводит billing-ошибку в доменную ошибку generation.
func (a generationBillingLimits) EnsureGenerationAllowed(ctx context.Context, userID string, requestedCards int) error {
	if a.billing == nil {
		return nil
	}

	err := a.billing.EnsureGenerationAllowed(ctx, userID, requestedCards)
	if err == nil {
		return nil
	}
	if errors.Is(err, billing.ErrGenerationLimitExceeded) {
		return generation.ErrQuotaExceeded
	}

	return err
}

// App хранит собранные runtime-зависимости приложения.
type App struct {
	Server *httpserver.Server
	DB     *postgres.Client
	Redis  *redis.Client
	Asynq  *jobs.Client
	Worker *jobs.Server
}

// New собирает приложение и проверяет доступность обязательной инфраструктуры.
func New(cfg config.Config, logger zerolog.Logger) (*App, error) {
	db, err := postgres.New(cfg.Postgres.DSN)
	if err != nil {
		return nil, fmt.Errorf("init postgres: %w", err)
	}

	redisClient, err := redis.New(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("init redis: %w", err)
	}

	asynqClient := jobs.New(redisClient.AsynqOpt(), cfg.Asynq.Concurrency)
	storageClient, err := storage.New(cfg.Storage.RootDir, cfg.Storage.PublicPath)
	if err != nil {
		db.Close()
		_ = redisClient.Close()
		_ = asynqClient.Close()
		return nil, fmt.Errorf("init storage: %w", err)
	}
	readiness := health.NewService(
		health.NewChecker("postgres", db.Ping),
		health.NewChecker("redis", redisClient.Ping),
	)
	healthHandler := handlers.NewHealthHandler(readiness, logger)
	queries := dbgen.New(db.Pool)
	// Если SMTP_HOST задан — используем реальный отправитель.
	// Иначе NoopSender: токен выводится в лог для локальной разработки.
	var emailSender auth.EmailSender
	if cfg.Email.Host != "" {
		emailSender = email.NewSMTPSender(cfg.Email, cfg.App.FrontendURL)
	} else {
		emailSender = email.NewNoopSender(logger)
	}
	authService := auth.NewService(db.Pool, queries, redisClient, emailSender, logger, cfg.Auth)
	authHandler := handlers.NewAuthHandler(authService)

	projectService := projects.NewService(queries)
	blogService := blog.NewService(queries)
	billingService := billing.NewService(queries)
	generationService := generation.NewService(
		db.Pool,
		queries,
		asynqClient,
		storageClient,
		generationBillingLimits{billing: billingService},
	)
	settingsService := settings.NewService(db.Pool, queries, asynqClient, authService.PasswordMinLength())
	dashboardHandler := handlers.NewDashboardHandler(projectService, logger)
	blogHandler := handlers.NewBlogHandler(blogService, logger)
	projectsHandler := handlers.NewProjectsHandler(projectService, logger)
	generationHandler := handlers.NewGenerationHandler(generationService, logger)
	billingHandler := handlers.NewBillingHandler(billingService, logger)
	settingsHandler := handlers.NewSettingsHandler(settingsService, logger)
	worker := jobs.NewServer(redisClient.AsynqOpt(), cfg.Asynq.Concurrency, logger, generationService)

	router := httptransport.NewRouter(
		cfg.HTTP,
		logger,
		healthHandler,
		authHandler,
		dashboardHandler,
		blogHandler,
		projectsHandler,
		generationHandler,
		billingHandler,
		settingsHandler,
		authService,
		cfg.Storage.PublicPath,
		storageClient.RootDir(),
	)
	server := httpserver.New(cfg.HTTP, router)

	return &App{
		Server: server,
		DB:     db,
		Redis:  redisClient,
		Asynq:  asynqClient,
		Worker: worker,
	}, nil
}

// Shutdown останавливает сервер и закрывает инфраструктурные клиенты.
func (a *App) Shutdown(ctx context.Context) error {
	var joined error

	if a.Server != nil {
		if err := a.Server.Shutdown(ctx); err != nil {
			joined = errors.Join(joined, err)
		}
	}

	// Worker останавливаем раньше Redis, чтобы фоновые задачи успели корректно завершить текущие операции.
	if a.Worker != nil {
		a.Worker.Shutdown()
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
