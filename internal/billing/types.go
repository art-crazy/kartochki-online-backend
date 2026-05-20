package billing

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"kartochki-online-backend/internal/dbgen"
)

// Billing описывает агрегированный ответ для страницы `/app/billing`.
type Billing struct {
	CurrentSubscription Subscription
	Plans               []Plan
	Addons              []Addon
}

// Subscription описывает текущую подписку пользователя и её usage.
type Subscription struct {
	PlanID           string
	PlanName         string
	RenewsAt         *time.Time
	CancelsAt        *time.Time
	HasPaymentMethod bool
	// PaymentMethod содержит данные привязанной карты. nil, если карта не сохранена.
	PaymentMethod *SavedPaymentMethod
	Usage         Usage
}

// SavedPaymentMethod описывает привязанный способ оплаты в подписке.
type SavedPaymentMethod struct {
	Type string
	Card *SavedCard
}

// SavedCard содержит реквизиты сохранённой банковской карты.
type SavedCard struct {
	Last4       string
	CardType    string
	ExpiryMonth string
	ExpiryYear  string
}

// Usage описывает текущее использование месячного лимита.
type Usage struct {
	Value int
	Max   int
}

// Plan описывает тариф, доступный на экране billing.
type Plan struct {
	ID                 string
	Name               string
	MonthlyPrice       int
	YearlyMonthlyPrice int
	CardsPerMonth      int
	Features           []PlanFeature
	Current            bool
	Popular            bool
}

// PlanFeature описывает одну возможность тарифа.
type PlanFeature struct {
	Label   string
	Enabled bool
}

// Addon описывает разовый пакет карточек.
type Addon struct {
	ID          string
	Title       string
	Description string
	Price       int
}

// PlanPeriod описывает период оплаты подписки.
type PlanPeriod string

const (
	// PlanPeriodMonthly описывает помесячную оплату тарифа.
	PlanPeriodMonthly PlanPeriod = "monthly"
	// PlanPeriodYearly описывает оплату тарифа за год.
	PlanPeriodYearly PlanPeriod = "yearly"
)

// CheckoutInput описывает запрос на оплату тарифа.
type CheckoutInput struct {
	UserID string
	PlanID string
	Period PlanPeriod
}

// CheckoutResult возвращает ссылку на hosted checkout.
type CheckoutResult struct {
	CheckoutURL string
}

// PurchaseAddonInput описывает покупку разового пакета карточек.
type PurchaseAddonInput struct {
	UserID  string
	AddonID string
}

// PurchaseAddonResult возвращает ссылку на checkout разового пакета.
type PurchaseAddonResult struct {
	CheckoutURL string
}

// Service управляет billing-сценариями поверх sqlc-запросов и checkout-провайдера.
type Service struct {
	pool     *pgxpool.Pool
	queries  *dbgen.Queries
	provider CheckoutProvider
}

// CheckoutProvider описывает внешний платёжный провайдер, который создаёт checkout-сессии.
// Реализация живёт в internal/platform/yookassa и подключается через app-адаптер.
type CheckoutProvider interface {
	CreateSubscriptionCheckout(ctx context.Context, input SubscriptionCheckoutInput) (CheckoutSession, error)
	CreateAddonCheckout(ctx context.Context, input AddonCheckoutInput) (CheckoutSession, error)
	CreateRecurringPayment(ctx context.Context, input RecurringPaymentInput) (CheckoutSession, error)
}

// CheckoutSession описывает созданный платёж у внешнего провайдера.
type CheckoutSession struct {
	ProviderPaymentID string
	CheckoutURL       string
}

// SubscriptionCheckoutInput описывает параметры checkout для покупки тарифа.
type SubscriptionCheckoutInput struct {
	UserID             string
	CustomerEmail      string
	PlanCode           string
	Period             PlanPeriod
	Amount             int
	Currency           string
	ReceiptDescription string
	// IdempotencyKey — стабильный ключ для дедупликации запроса на стороне платёжного провайдера.
	// Вычисляется в billing-сервисе, чтобы провайдер не создавал дублирующий платёж при повторных попытках.
	IdempotencyKey string
}

// AddonCheckoutInput описывает параметры checkout для покупки разового пакета.
type AddonCheckoutInput struct {
	UserID             string
	CustomerEmail      string
	AddonCode          string
	Amount             int
	Currency           string
	ReceiptDescription string
	// IdempotencyKey — стабильный ключ для дедупликации запроса на стороне платёжного провайдера.
	IdempotencyKey string
}

// RecurringPaymentInput описывает параметры автосписания за продление подписки.
type RecurringPaymentInput struct {
	UserID             string
	CustomerEmail      string
	PlanCode           string
	Period             PlanPeriod
	Amount             int
	Currency           string
	PaymentMethodID    string
	ReceiptDescription string
	IdempotencyKey     string
}

type noopCheckoutProvider struct{}

var _ CheckoutProvider = noopCheckoutProvider{}
