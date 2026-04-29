package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"kartochki-online-backend/internal/billing"
	"kartochki-online-backend/internal/http/requestctx"
	"kartochki-online-backend/internal/http/response"
	"kartochki-online-backend/internal/platform/yookassa"
)

const webhookBodyLimit = 1 << 20 // 1 MB

// webhookBillingService описывает минимальный контракт billing-сервиса для webhook handler.
type webhookBillingService interface {
	HandleWebhookEvent(ctx context.Context, event billing.WebhookEvent) error
}

// PaymentStatusVerifier проверяет актуальное состояние платежа у провайдера.
type PaymentStatusVerifier interface {
	GetPayment(ctx context.Context, paymentID string) (yookassa.PaymentObject, error)
}

// NoopWebhookVerifier доверяет payload без запроса к провайдеру.
// Используется только в локальной разработке, когда ЮКасса не настроена.
type NoopWebhookVerifier struct{}

// GetPayment возвращает пустой объект: handler оставит payload как есть.
func (NoopWebhookVerifier) GetPayment(context.Context, string) (yookassa.PaymentObject, error) {
	return yookassa.PaymentObject{}, nil
}

// BillingWebhookHandler обслуживает POST /api/v1/billing/webhook.
type BillingWebhookHandler struct {
	service  webhookBillingService
	verifier PaymentStatusVerifier
	logger   zerolog.Logger
}

// NewBillingWebhookHandler создаёт handler для приёма webhook-уведомлений от ЮКасса.
func NewBillingWebhookHandler(
	service webhookBillingService,
	verifier PaymentStatusVerifier,
	logger zerolog.Logger,
) BillingWebhookHandler {
	return BillingWebhookHandler{
		service:  service,
		verifier: verifier,
		logger:   logger,
	}
}

// Handle принимает webhook-уведомление от ЮКасса, сверяет статус платежа и вызывает billing-сервис.
// ЮКасса повторяет уведомление при ответах, отличных от 200, поэтому метод идемпотентен.
func (h BillingWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	logger := requestctx.Logger(r.Context(), h.logger)

	// Читаем тело целиком до обработки, чтобы корректно разобрать событие и ограничить размер payload.
	body, err := io.ReadAll(io.LimitReader(r.Body, webhookBodyLimit))
	if err != nil {
		logger.Warn().Err(err).Msg("webhook: не удалось прочитать тело запроса")
		response.WriteError(w, r, http.StatusBadRequest, "invalid_request", "failed to read request body")
		return
	}

	raw, err := yookassa.ParseWebhookEvent(body)
	if err != nil {
		logger.Warn().Err(err).Msg("webhook: не удалось распарсить событие")
		response.WriteError(w, r, http.StatusBadRequest, "invalid_payload", "failed to parse webhook event")
		return
	}

	logger.Info().
		Str("event_type", string(raw.Type)).
		Str("payment_id", raw.Object.ID).
		Msg("webhook: получено событие")

	if raw.Object.ID == "" {
		logger.Warn().Str("event_type", string(raw.Type)).Msg("webhook: пустой payment_id в событии")
		response.WriteError(w, r, http.StatusBadRequest, "invalid_payload", "webhook event missing payment id")
		return
	}

	verifiedPayment, err := h.verifier.GetPayment(r.Context(), raw.Object.ID)
	if err != nil {
		logger.Error().
			Err(err).
			Str("payment_id", raw.Object.ID).
			Msg("webhook: не удалось проверить статус платежа в ЮКасса")
		response.WriteError(w, r, http.StatusBadGateway, "provider_unavailable", "failed to verify payment status")
		return
	}
	if verifiedPayment.ID != "" {
		raw.Object = verifiedPayment
		if !eventMatchesPaymentStatus(raw.Type, raw.Object.Status) {
			logger.Warn().
				Str("event_type", string(raw.Type)).
				Str("payment_id", raw.Object.ID).
				Str("provider_status", raw.Object.Status).
				Msg("webhook: статус платежа не совпадает с типом события")
			response.WriteError(w, r, http.StatusBadRequest, "invalid_payload", "webhook event does not match provider payment status")
			return
		}
	}

	event, err := yookassaEventToBilling(raw)
	if err != nil {
		logger.Warn().
			Err(err).
			Str("event_type", string(raw.Type)).
			Str("payment_id", raw.Object.ID).
			Msg("webhook: не удалось конвертировать событие")
		response.WriteError(w, r, http.StatusBadRequest, "invalid_payload", "failed to parse webhook event fields")
		return
	}

	if err := h.service.HandleWebhookEvent(r.Context(), event); err != nil {
		logger.Error().
			Err(err).
			Str("event_type", string(raw.Type)).
			Str("payment_id", raw.Object.ID).
			Msg("webhook: ошибка обработки события")
		// Возвращаем 500, чтобы ЮКасса повторила уведомление позже.
		response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to handle webhook event")
		return
	}

	// ЮКасса ожидает любой 2xx ответ для подтверждения получения.
	response.WriteJSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
}

func eventMatchesPaymentStatus(eventType yookassa.EventType, status string) bool {
	switch eventType {
	case yookassa.EventPaymentSucceeded:
		return status == "succeeded"
	case yookassa.EventPaymentCanceled:
		return status == "canceled"
	default:
		return true
	}
}

// yookassaEventToBilling конвертирует yookassa.WebhookEvent в доменный billing.WebhookEvent.
// Конвертация живёт в transport-слое, чтобы billing-домен не зависел от деталей провайдера.
func yookassaEventToBilling(raw yookassa.WebhookEvent) (billing.WebhookEvent, error) {
	// Нормализуем строки на входе, чтобы внутренняя логика billing не зависела от пробелов.
	event := billing.WebhookEvent{
		ProviderPaymentID:       strings.TrimSpace(raw.Object.ID),
		EventType:               billing.WebhookEventType(raw.Type),
		ProviderPaymentMethodID: strings.TrimSpace(raw.Object.PaymentMethod.ID),
		Amount: billing.WebhookPaymentAmount{
			Value:    strings.TrimSpace(raw.Object.Amount.Value),
			Currency: strings.TrimSpace(raw.Object.Amount.Currency),
		},
		Metadata: billing.WebhookPaymentMetadata{
			UserID:    strings.TrimSpace(raw.Object.Metadata.UserID),
			PlanCode:  strings.TrimSpace(raw.Object.Metadata.PlanCode),
			AddonCode: strings.TrimSpace(raw.Object.Metadata.AddonCode),
			Period:    strings.TrimSpace(raw.Object.Metadata.Period),
			Type:      strings.TrimSpace(raw.Object.Metadata.Type),
		},
	}

	if raw.Object.PaidAt != "" {
		t, err := time.Parse(time.RFC3339, raw.Object.PaidAt)
		if err != nil {
			return billing.WebhookEvent{}, fmt.Errorf("parse captured_at %q: %w", raw.Object.PaidAt, err)
		}
		event.PaidAt = &t
	}

	return event, nil
}
