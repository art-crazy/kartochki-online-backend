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
	"kartochki-online-backend/internal/platform/routerai"
	"kartochki-online-backend/internal/platform/storage"
	"kartochki-online-backend/internal/platform/yookassa"
	"kartochki-online-backend/internal/projects"
	"kartochki-online-backend/internal/settings"
)

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
	authService := auth.NewService(db.Pool, queries, asynqClient, logger, cfg.Auth)
	authHandler := handlers.NewAuthHandler(authService, cfg.App.IsProduction(), cfg.App.FrontendURL)

	projectService := projects.NewService(queries)
	blogService := blog.NewService(queries)

	// Если YOOKASSA_SHOP_ID задан — используем реальный клиент ЮКасса.
	// Иначе noopCheckoutProvider: checkout вернёт ошибку, но остальной billing работает,
	// а проверка подписи webhook пропускается (NoopWebhookVerifier).
	var (
		billingProvider billing.CheckoutProvider
		webhookVerifier handlers.WebhookSignatureVerifier = handlers.NoopWebhookVerifier{}
	)
	if cfg.YooKassa.ShopID != "" {
		yk := yookassa.New(cfg.YooKassa)
		billingProvider = yookassaCheckoutAdapter{client: yk}
		webhookVerifier = yk
	}
	billingService := billing.NewService(db.Pool, queries, billingProvider)

	// Если ROUTERAI_API_KEY задан — используем реальный генератор изображений.
	// Иначе передаём nil: generation.NewService подставит noopImageGenerator,
	// который возвращает ошибку, и генерация будет явно падать в статус failed.
	var imageGen generation.ImageGenerator
	if cfg.RouterAI.APIKey != "" {
		imageGen = routerAIAdapter{client: routerai.New(cfg.RouterAI)}
	}

	generationService := generation.NewService(
		db.Pool,
		queries,
		asynqGenerationEnqueuer{client: asynqClient},
		storageClient,
		generationBillingLimits{billing: billingService},
		imageGen,
	)
	settingsService := settings.NewService(db.Pool, queries, asynqClient, authService.PasswordMinLength())
	dashboardHandler := handlers.NewDashboardHandler(projectService, logger)
	blogHandler := handlers.NewBlogHandler(blogService, logger)
	projectsHandler := handlers.NewProjectsHandler(projectService, logger)
	generationHandler := handlers.NewGenerationHandler(generationService, logger)
	billingHandler := handlers.NewBillingHandler(billingService, logger)
	billingWebhookHandler := handlers.NewBillingWebhookHandler(billingService, webhookVerifier, logger)
	settingsHandler := handlers.NewSettingsHandler(settingsService, logger)
	worker := jobs.NewServer(redisClient.AsynqOpt(), cfg.Asynq.Concurrency, logger, generationWorkerAdapter{service: generationService}, authEmailWorker{
		sender:      emailSender,
		sendTimeout: cfg.Auth.EmailSendTimeout,
		logger:      logger,
	})

	router := httptransport.NewRouter(
		cfg.HTTP,
		cfg.AuthRateLimit,
		logger,
		healthHandler,
		authHandler,
		dashboardHandler,
		blogHandler,
		projectsHandler,
		generationHandler,
		billingHandler,
		billingWebhookHandler,
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
