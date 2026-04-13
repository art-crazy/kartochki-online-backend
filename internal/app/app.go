package app

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// Статические проверки соответствия адаптеров своим интерфейсам.
// Компилятор сообщит об ошибке, если сигнатура метода разойдётся с интерфейсом.
var _ jobs.SendPasswordResetEmailHandler = authEmailWorker{}
var _ billing.CheckoutProvider = yookassaCheckoutAdapter{}
var _ handlers.WebhookSignatureVerifier = (*yookassa.Client)(nil)
var _ generation.ImageGenerator = routerAIAdapter{}

// routerAIAdapter оборачивает routerai.Client и реализует generation.ImageGenerator.
// Живёт в app-пакете, чтобы generation и routerai не зависели друг от друга.
type routerAIAdapter struct {
	client *routerai.Client
}

// GenerateImage делегирует вызов routerai.Client, конвертируя доменный тип входных данных.
func (a routerAIAdapter) GenerateImage(ctx context.Context, input generation.ImageGenerateInput) ([]byte, error) {
	return a.client.GenerateImage(ctx, routerai.GenerateImageInput{
		Prompt:      input.Prompt,
		AspectRatio: input.AspectRatio,
		ModelID:     input.ModelID,
	})
}

// yookassaCheckoutAdapter оборачивает yookassa.Client и реализует billing.CheckoutProvider.
// Живёт в app-пакете, чтобы ни billing, ни yookassa не зависели друг от друга.
type yookassaCheckoutAdapter struct {
	client *yookassa.Client
}

// CreateSubscriptionCheckout реализует billing.CheckoutProvider для подписки.
func (a yookassaCheckoutAdapter) CreateSubscriptionCheckout(ctx context.Context, input billing.SubscriptionCheckoutInput) (string, error) {
	return a.client.CreateSubscriptionCheckout(ctx, yookassa.SubscriptionCheckoutInput{
		UserID:         input.UserID,
		PlanCode:       input.PlanCode,
		Period:         string(input.Period),
		Amount:         input.Amount,
		Currency:       input.Currency,
		IdempotencyKey: input.IdempotencyKey,
	})
}

// CreateAddonCheckout реализует billing.CheckoutProvider для разового пакета.
func (a yookassaCheckoutAdapter) CreateAddonCheckout(ctx context.Context, input billing.AddonCheckoutInput) (string, error) {
	return a.client.CreateAddonCheckout(ctx, yookassa.AddonCheckoutInput{
		UserID:         input.UserID,
		AddonCode:      input.AddonCode,
		Amount:         input.Amount,
		Currency:       input.Currency,
		IdempotencyKey: input.IdempotencyKey,
	})
}

// authEmailWorker адаптирует auth.EmailSender к интерфейсу jobs.SendPasswordResetEmailHandler.
// Обёртка живёт в app-пакете, чтобы ни auth, ни jobs не зависели друг от друга.
type authEmailWorker struct {
	sender      auth.EmailSender
	sendTimeout time.Duration
	logger      zerolog.Logger
}

// HandleSendPasswordResetEmail вызывается Asynq worker-ом при обработке задачи отправки письма.
// При ошибке возвращает её явно: Asynq повторит задачу согласно MaxRetry.
func (w authEmailWorker) HandleSendPasswordResetEmail(ctx context.Context, payload jobs.SendPasswordResetEmailPayload) error {
	sendCtx, cancel := context.WithTimeout(ctx, w.sendTimeout)
	defer cancel()

	if err := w.sender.SendPasswordResetEmail(sendCtx, payload.Email, payload.RawToken); err != nil {
		w.logger.Error().Err(err).
			Str("user_id", payload.UserID).
			Str("email", payload.Email).
			Msg("worker: не удалось отправить письмо для сброса пароля")
		return err
	}

	w.logger.Info().
		Str("user_id", payload.UserID).
		Str("email", payload.Email).
		Msg("worker: письмо для сброса пароля отправлено")

	return nil
}

// generationBillingLimits адаптирует billing-сервис к минимальному контракту generation.
// Так generation не знает о деталях billing-домена и зависит только от проверяемого правила.
type generationBillingLimits struct {
	billing *billing.Service
}

// EnsureGenerationAllowed проверяет квоту и переводит billing-ошибки в доменные ошибки generation.
// Известные billing-ошибки отображаются в конкретные generation-ошибки, чтобы HTTP-слой
// не зависел от billing-пакета и мог корректно формировать ответ.
// Неожиданные ошибки оборачиваются с пометкой "billing check", чтобы в логе было видно источник.
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

	// Неожиданная ошибка из billing-сервиса: оборачиваем с контекстом,
	// чтобы в логе был виден источник, а handler вернул 500 с понятным сообщением.
	return fmt.Errorf("billing check failed: %w", err)
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
	authService := auth.NewService(db.Pool, queries, redisClient, asynqClient, logger, cfg.Auth)
	authHandler := handlers.NewAuthHandler(authService)

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
		asynqClient,
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
	worker := jobs.NewServer(redisClient.AsynqOpt(), cfg.Asynq.Concurrency, logger, generationService, authEmailWorker{
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
