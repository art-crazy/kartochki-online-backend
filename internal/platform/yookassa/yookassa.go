// Package yookassa реализует HTTP-клиент для платёжной системы ЮКасса.
// Адаптер реализует интерфейс billing.checkoutProvider и используется как инфраструктурная зависимость.
// Документация API: https://yookassa.ru/developers/api
package yookassa

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kartochki-online-backend/internal/config"
)

const (
	baseURL         = "https://api.yookassa.ru/v3"
	defaultTimeout  = 15 * time.Second
	maxRespBodySize = 1 << 20 // 1 MB — защита от аномально большого ответа
)

// EventType описывает тип события из webhook-уведомления ЮКасса.
type EventType string

const (
	// EventPaymentSucceeded — платёж успешно завершён.
	EventPaymentSucceeded EventType = "payment.succeeded"
	// EventPaymentCanceled — платёж отменён или истёк срок ожидания.
	EventPaymentCanceled EventType = "payment.canceled"
)

// Client — HTTP-клиент ЮКасса.
// Аутентификация — Basic Auth: shopID как логин, secretKey как пароль.
type Client struct {
	shopID        string
	secretKey     string
	webhookSecret string
	returnURL     string
	httpClient    *http.Client
}

// New создаёт клиент ЮКасса из конфига.
func New(cfg config.YooKassaConfig) *Client {
	return &Client{
		shopID:        cfg.ShopID,
		secretKey:     cfg.SecretKey,
		webhookSecret: cfg.WebhookSecret,
		returnURL:     cfg.ReturnURL,
		httpClient:    &http.Client{Timeout: defaultTimeout},
	}
}

// CreateSubscriptionCheckout создаёт платёж для покупки тарифа и возвращает URL страницы оплаты.
// Параметр save_payment_method позволит повторно списывать деньги при продлении.
func (c *Client) CreateSubscriptionCheckout(ctx context.Context, input SubscriptionCheckoutInput) (string, error) {
	amountStr := formatAmount(input.Amount)

	body := map[string]any{
		"amount": map[string]string{
			"value":    amountStr,
			"currency": input.Currency,
		},
		"confirmation": map[string]string{
			"type":       "redirect",
			"return_url": c.returnURL,
		},
		"capture":             true,
		"save_payment_method": true,
		"description":         fmt.Sprintf("Подписка %s (%s)", input.PlanCode, input.Period),
		"metadata": map[string]string{
			"user_id":   input.UserID,
			"plan_code": input.PlanCode,
			"period":    string(input.Period),
			"type":      "subscription",
		},
	}

	return c.createPayment(ctx, body, input.IdempotencyKey)
}

// CreateAddonCheckout создаёт разовый платёж для покупки пакета карточек.
func (c *Client) CreateAddonCheckout(ctx context.Context, input AddonCheckoutInput) (string, error) {
	amountStr := formatAmount(input.Amount)

	body := map[string]any{
		"amount": map[string]string{
			"value":    amountStr,
			"currency": input.Currency,
		},
		"confirmation": map[string]string{
			"type":       "redirect",
			"return_url": c.returnURL,
		},
		"capture":     true,
		"description": fmt.Sprintf("Пакет карточек: %s", input.AddonCode),
		"metadata": map[string]string{
			"user_id":    input.UserID,
			"addon_code": input.AddonCode,
			"type":       "addon",
		},
	}

	return c.createPayment(ctx, body, input.IdempotencyKey)
}

// VerifyWebhookSignature проверяет подпись входящего webhook-уведомления.
// ЮКасса передаёт HMAC-SHA256 подпись в заголовке YooKassa-Signature.
// При пустом webhookSecret проверка пропускается — только для локальной разработки.
func (c *Client) VerifyWebhookSignature(body []byte, signature string) bool {
	if c.webhookSecret == "" {
		return true
	}

	mac := hmac.New(sha256.New, []byte(c.webhookSecret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(strings.ToLower(signature)))
}

// ParseWebhookEvent разбирает тело webhook-уведомления от ЮКасса.
func ParseWebhookEvent(body []byte) (WebhookEvent, error) {
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return WebhookEvent{}, fmt.Errorf("parse yookassa webhook: %w", err)
	}

	return event, nil
}


// createPayment отправляет запрос на создание платежа и возвращает confirmation_url.
func (c *Client) createPayment(ctx context.Context, body map[string]any, idempotencyKey string) (string, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal yookassa payment request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/payments", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("build yookassa request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotence-Key", idempotencyKey)
	req.SetBasicAuth(c.shopID, c.secretKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("yookassa http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBodySize))
	if err != nil {
		return "", fmt.Errorf("read yookassa response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("yookassa returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result createPaymentResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse yookassa payment response: %w", err)
	}

	if result.Confirmation.ConfirmationURL == "" {
		return "", fmt.Errorf("yookassa returned empty confirmation_url")
	}

	return result.Confirmation.ConfirmationURL, nil
}

// formatAmount переводит целую сумму в копейках в строку рублей с двумя знаками после запятой.
// ЮКасса ожидает сумму в формате "100.00" (рубли).
func formatAmount(kopecks int) string {
	rubles := kopecks / 100
	cents := kopecks % 100
	return fmt.Sprintf("%d.%02d", rubles, cents)
}

// SubscriptionCheckoutInput — параметры для создания платежа подписки.
type SubscriptionCheckoutInput struct {
	UserID         string
	PlanCode       string
	Period         string
	Amount         int
	Currency       string
	IdempotencyKey string
}

// AddonCheckoutInput — параметры для создания разового платежа addon.
type AddonCheckoutInput struct {
	UserID         string
	AddonCode      string
	Amount         int
	Currency       string
	IdempotencyKey string
}

// WebhookEvent описывает входящее уведомление от ЮКасса.
type WebhookEvent struct {
	// Type — тип события, например "payment.succeeded".
	Type   EventType      `json:"type"`
	Object PaymentObject  `json:"object"`
}

// PaymentObject описывает объект платежа внутри webhook-уведомления.
type PaymentObject struct {
	ID          string          `json:"id"`
	Status      string          `json:"status"`
	Amount      PaymentAmount   `json:"amount"`
	Description string          `json:"description"`
	Metadata    PaymentMetadata `json:"metadata"`
	// PaidAt — время фактического списания средств.
	PaidAt      string `json:"captured_at"`
	// ExpiresAt — время истечения авторизации (для двухэтапных платежей).
	ExpiresAt   string `json:"expires_at"`
	// PaymentMethodID сохраняется ЮКасса при save_payment_method: true.
	// Используется для рекуррентных списаний при продлении подписки.
	PaymentMethodID string `json:"payment_method_id"`
}

// PaymentAmount описывает сумму платежа.
type PaymentAmount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

// PaymentMetadata хранит произвольные поля, переданные при создании платежа.
type PaymentMetadata struct {
	UserID    string `json:"user_id"`
	PlanCode  string `json:"plan_code"`
	AddonCode string `json:"addon_code"`
	Period    string `json:"period"`
	// Type — тип платежа: "subscription" или "addon".
	Type string `json:"type"`
}

type createPaymentResponse struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	Confirmation struct {
		ConfirmationURL string `json:"confirmation_url"`
	} `json:"confirmation"`
}
