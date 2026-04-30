package yookassa

// CheckoutSession содержит данные созданного платежа ЮКасса.
type CheckoutSession struct {
	ProviderPaymentID string
	CheckoutURL       string
}

// SubscriptionCheckoutInput — параметры для создания платежа подписки.
type SubscriptionCheckoutInput struct {
	UserID             string
	CustomerEmail      string
	PlanCode           string
	Period             string
	Amount             int
	Currency           string
	ReceiptDescription string
	IdempotencyKey     string
}

// AddonCheckoutInput — параметры для создания разового платежа addon.
type AddonCheckoutInput struct {
	UserID             string
	CustomerEmail      string
	AddonCode          string
	Amount             int
	Currency           string
	ReceiptDescription string
	IdempotencyKey     string
}

// RecurringPaymentInput — параметры для рекуррентного платежа без confirmation.
type RecurringPaymentInput struct {
	UserID          string
	PlanCode        string
	Period          string
	Amount          int
	Currency        string
	PaymentMethodID string
	IdempotencyKey  string
}

// WebhookEvent описывает входящее уведомление от ЮКасса.
type WebhookEvent struct {
	// Type — тип события, например "payment.succeeded".
	Type   EventType     `json:"type"`
	Object PaymentObject `json:"object"`
}

// PaymentObject описывает объект платежа внутри webhook-уведомления.
type PaymentObject struct {
	ID          string          `json:"id"`
	Status      string          `json:"status"`
	Amount      PaymentAmount   `json:"amount"`
	Description string          `json:"description"`
	Metadata    PaymentMetadata `json:"metadata"`
	// PaidAt — время фактического списания средств.
	PaidAt string `json:"captured_at"`
	// ExpiresAt — время истечения авторизации (для двухэтапных платежей).
	ExpiresAt string `json:"expires_at"`
	// PaymentMethod содержит сохранённый способ оплаты при save_payment_method: true.
	// Его id нужен для будущих рекуррентных списаний.
	PaymentMethod PaymentMethod `json:"payment_method"`
}

// PaymentMethod описывает способ оплаты внутри объекта платежа ЮКасса.
type PaymentMethod struct {
	ID    string `json:"id"`
	Saved bool   `json:"saved"`
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
	// Type — тип платежа: "subscription", "subscription_renewal" или "addon".
	Type string `json:"type"`
}
