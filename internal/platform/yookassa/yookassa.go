// Package yookassa реализует HTTP-клиент для платёжной системы ЮКасса.
// Адаптер реализует интерфейс billing.checkoutProvider и используется как инфраструктурная зависимость.
// Документация API: https://yookassa.ru/developers/api
package yookassa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	shopID     string
	secretKey  string
	returnURL  string
	httpClient *http.Client
}

// New создаёт клиент ЮКасса из конфига.
func New(cfg config.YooKassaConfig) *Client {
	return &Client{
		shopID:     cfg.ShopID,
		secretKey:  cfg.SecretKey,
		returnURL:  cfg.ReturnURL,
		httpClient: &http.Client{Timeout: defaultTimeout},
	}
}

// CreateSubscriptionCheckout создаёт платёж для покупки тарифа и возвращает данные checkout.
// Параметр save_payment_method позволит повторно списывать деньги при продлении.
func (c *Client) CreateSubscriptionCheckout(ctx context.Context, input SubscriptionCheckoutInput) (CheckoutSession, error) {
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

	return c.createPayment(ctx, body, input.IdempotencyKey, true)
}

// CreateAddonCheckout создаёт разовый платёж для покупки пакета карточек.
func (c *Client) CreateAddonCheckout(ctx context.Context, input AddonCheckoutInput) (CheckoutSession, error) {
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

	return c.createPayment(ctx, body, input.IdempotencyKey, true)
}

// CreateRecurringPayment создаёт автосписание по сохранённому payment_method_id без участия пользователя.
func (c *Client) CreateRecurringPayment(ctx context.Context, input RecurringPaymentInput) (CheckoutSession, error) {
	body := map[string]any{
		"amount": map[string]string{
			"value":    formatAmount(input.Amount),
			"currency": input.Currency,
		},
		"capture":           true,
		"payment_method_id": input.PaymentMethodID,
		"description":       fmt.Sprintf("Продление подписки %s (%s)", input.PlanCode, input.Period),
		"metadata": map[string]string{
			"user_id":   input.UserID,
			"plan_code": input.PlanCode,
			"period":    string(input.Period),
			"type":      "subscription_renewal",
		},
	}

	return c.createPayment(ctx, body, input.IdempotencyKey, false)
}

// ParseWebhookEvent разбирает тело webhook-уведомления от ЮКасса.
func ParseWebhookEvent(body []byte) (WebhookEvent, error) {
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return WebhookEvent{}, fmt.Errorf("parse yookassa webhook: %w", err)
	}

	return event, nil
}

// GetPayment получает актуальное состояние платежа в ЮКасса.
// Это используется для проверки webhook: событие принимаем только после сверки статуса у провайдера.
func (c *Client) GetPayment(ctx context.Context, paymentID string) (PaymentObject, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/payments/"+url.PathEscape(paymentID), nil)
	if err != nil {
		return PaymentObject{}, fmt.Errorf("build yookassa get payment request: %w", err)
	}

	req.SetBasicAuth(c.shopID, c.secretKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return PaymentObject{}, fmt.Errorf("yookassa get payment request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBodySize))
	if err != nil {
		return PaymentObject{}, fmt.Errorf("read yookassa get payment response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return PaymentObject{}, fmt.Errorf("yookassa get payment returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var payment PaymentObject
	if err := json.Unmarshal(respBody, &payment); err != nil {
		return PaymentObject{}, fmt.Errorf("parse yookassa get payment response: %w", err)
	}

	return payment, nil
}

// createPayment отправляет запрос на создание платежа и возвращает данные для сохранения в БД.
func (c *Client) createPayment(ctx context.Context, body map[string]any, idempotencyKey string, requireConfirmation bool) (CheckoutSession, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return CheckoutSession{}, fmt.Errorf("marshal yookassa payment request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/payments", bytes.NewReader(data))
	if err != nil {
		return CheckoutSession{}, fmt.Errorf("build yookassa request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotence-Key", idempotencyKey)
	req.SetBasicAuth(c.shopID, c.secretKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return CheckoutSession{}, fmt.Errorf("yookassa http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBodySize))
	if err != nil {
		return CheckoutSession{}, fmt.Errorf("read yookassa response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return CheckoutSession{}, fmt.Errorf("yookassa returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result createPaymentResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return CheckoutSession{}, fmt.Errorf("parse yookassa payment response: %w", err)
	}

	if result.ID == "" {
		return CheckoutSession{}, fmt.Errorf("yookassa returned empty payment id")
	}
	if requireConfirmation && result.Confirmation.ConfirmationURL == "" {
		return CheckoutSession{}, fmt.Errorf("yookassa returned empty confirmation_url")
	}

	return CheckoutSession{
		ProviderPaymentID: result.ID,
		CheckoutURL:       result.Confirmation.ConfirmationURL,
	}, nil
}

// formatAmount переводит целую сумму в рублях в строку с двумя знаками после запятой.
// ЮКасса ожидает сумму в формате "100.00" (рубли).
func formatAmount(rubles int) string {
	return fmt.Sprintf("%d.00", rubles)
}

type createPaymentResponse struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	Confirmation struct {
		ConfirmationURL string `json:"confirmation_url"`
	} `json:"confirmation"`
}
