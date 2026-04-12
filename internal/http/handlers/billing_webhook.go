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

// WebhookSignatureVerifier проверяет подпись входящего webhook-уведомления.
type WebhookSignatureVerifier interface {
	VerifyWebhookSignature(body []byte, signature string) bool
}

// NoopWebhookVerifier всегда возвращает true — используется когда провайдер не настроен.
type NoopWebhookVerifier struct{}

// VerifyWebhookSignature всегда возвращает true (проверка отключена).
func (NoopWebhookVerifier) VerifyWebhookSignature([]byte, string) bool { return true }

// BillingWebhookHandler обслуживает POST /api/v1/billing/webhook.
type BillingWebhookHandler struct {
	service  webhookBillingService
	verifier WebhookSignatureVerifier
	logger   zerolog.Logger
}

// NewBillingWebhookHandler создаёт handler для приёма webhook-уведомлений от ЮКасса.
func NewBillingWebhookHandler(
	service webhookBillingService,
	verifier WebhookSignatureVerifier,
	logger zerolog.Logger,
) BillingWebhookHandler {
	return BillingWebhookHandler{
		service:  service,
		verifier: verifier,
		logger:   logger,
	}
}

// Handle принимает webhook-уведомление от ЮКасса, проверяет подпись и вызывает billing-сервис.
// ЮКасса повторяет уведомление при ответах, отличных от 200, поэтому метод идемпотентен.
func (h BillingWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	logger := requestctx.Logger(r.Context(), h.logger)

	// Читаем тело целиком до обработки, чтобы проверить подпись по сырым байтам.
	body, err := io.ReadAll(io.LimitReader(r.Body, webhookBodyLimit))
	if err != nil {
		logger.Warn().Err(err).Msg("webhook: не удалось прочитать тело запроса")
		response.WriteError(w, r, http.StatusBadRequest, "invalid_request", "failed to read request body")
		return
	}

	// Проверяем подпись из заголовка YooKassa-Signature.
	// При пустом YOOKASSA_WEBHOOK_SECRET проверка пропускается (только для локальной разработки).
	signature := r.Header.Get("YooKassa-Signature")
	if !h.verifier.VerifyWebhookSignature(body, signature) {
		logger.Warn().Str("signature", signature).Msg("webhook: неверная подпись")
		response.WriteError(w, r, http.StatusUnauthorized, "invalid_signature", "webhook signature is invalid")
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

// yookassaEventToBilling конвертирует yookassa.WebhookEvent в доменный billing.WebhookEvent.
// Конвертация живёт в transport-слое, чтобы billing-домен не зависел от деталей провайдера.
func yookassaEventToBilling(raw yookassa.WebhookEvent) (billing.WebhookEvent, error) {
	// Нормализуем строки на входе, чтобы внутренняя логика billing не зависела от пробелов.
	event := billing.WebhookEvent{
		ProviderPaymentID: strings.TrimSpace(raw.Object.ID),
		EventType:         billing.WebhookEventType(raw.Type),
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
