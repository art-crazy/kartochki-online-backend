package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	"kartochki-online-backend/internal/auth"
	"kartochki-online-backend/internal/billing"
	"kartochki-online-backend/internal/generation"
	"kartochki-online-backend/internal/http/handlers"
	"kartochki-online-backend/internal/jobs"
	"kartochki-online-backend/internal/platform/routerai"
	"kartochki-online-backend/internal/platform/yookassa"
)

// Статические проверки соответствия адаптеров своим интерфейсам.
// Компилятор сообщит об ошибке, если сигнатура метода разойдётся с интерфейсом.
var _ jobs.SendAuthEmailHandler = authEmailWorker{}
var _ billing.CheckoutProvider = yookassaCheckoutAdapter{}
var _ handlers.WebhookSignatureVerifier = (*yookassa.Client)(nil)
var _ generation.ImageGenerator = routerAIAdapter{}
var _ generation.GenerationJobEnqueuer = asynqGenerationEnqueuer{}
var _ jobs.GenerationHandler = generationWorkerAdapter{}

// asynqGenerationEnqueuer адаптирует jobs.Client к минимальному контракту generation.
// Так generation-сервис не зависит от конкретного клиента фоновой очереди.
type asynqGenerationEnqueuer struct {
	client *jobs.Client
}

// EnqueueGeneration ставит задачу генерации в Asynq по id generation.
func (e asynqGenerationEnqueuer) EnqueueGeneration(ctx context.Context, generationID string) error {
	if e.client == nil {
		return errors.New("asynq client is not configured")
	}
	_, err := e.client.EnqueueGeneration(ctx, jobs.GenerationPayload{GenerationID: generationID})
	return err
}

// generationWorkerAdapter адаптирует generation.Service к Asynq worker-контракту.
// Это оставляет пакет generation без импорта internal/jobs.
type generationWorkerAdapter struct {
	service *generation.Service
}

// HandleGeneration извлекает payload очереди и передаёт домену только id generation.
func (a generationWorkerAdapter) HandleGeneration(ctx context.Context, payload jobs.GenerationPayload) error {
	return a.service.HandleGeneration(ctx, payload.GenerationID)
}

// routerAIAdapter оборачивает routerai.Client и реализует generation.ImageGenerator.
// Живёт в app-пакете, чтобы generation и routerai не зависели друг от друга.
type routerAIAdapter struct {
	client *routerai.Client
}

// GenerateImage делегирует вызов routerai.Client, конвертируя доменный тип входных данных.
func (a routerAIAdapter) GenerateImage(ctx context.Context, input generation.ImageGenerateInput) ([]byte, error) {
	return a.client.GenerateImage(ctx, routerai.GenerateImageInput{
		Prompt:              input.Prompt,
		SourceImageBody:     input.SourceImageBody,
		SourceImageMIMEType: input.SourceImageMIMEType,
		AspectRatio:         input.AspectRatio,
		ModelID:             input.ModelID,
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

// authEmailWorker адаптирует auth.EmailSender к worker-контракту auth-писем.
// Обёртка живёт в app-пакете, чтобы ни auth, ни jobs не зависели друг от друга.
type authEmailWorker struct {
	sender      auth.EmailSender
	sendTimeout time.Duration
	logger      zerolog.Logger
}

// HandleSendAuthEmail вызывается Asynq worker-ом при обработке auth-письма.
func (w authEmailWorker) HandleSendAuthEmail(ctx context.Context, payload jobs.SendAuthEmailPayload) error {
	sendCtx, cancel := context.WithTimeout(ctx, w.sendTimeout)
	defer cancel()

	var err error
	switch payload.Kind {
	case jobs.AuthEmailKindPasswordReset:
		err = w.sender.SendPasswordResetEmail(sendCtx, payload.Email, payload.RawToken)
	case jobs.AuthEmailKindRegistrationVerification:
		err = w.sender.SendRegistrationVerificationEmail(sendCtx, payload.Email, payload.VerificationID, payload.Code, payload.ExpiresIn)
	default:
		return fmt.Errorf("unknown auth email kind: %s", payload.Kind)
	}
	if err != nil {
		w.logger.Error().Err(err).Str("user_id", payload.UserID).Str("email", payload.Email).Str("kind", payload.Kind).Msg("worker: не удалось отправить auth-письмо")
		return err
	}
	w.logger.Info().Str("user_id", payload.UserID).Str("email", payload.Email).Str("kind", payload.Kind).Msg("worker: auth-письмо отправлено")
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
