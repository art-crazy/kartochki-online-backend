package contracts

import "time"

// BillingResponse описывает данные для страницы `/app/billing`.
type BillingResponse struct {
	CurrentSubscription BillingSubscription `json:"current_subscription"`
	Plans               []BillingPlan       `json:"plans"`
	Addons              []BillingAddon      `json:"addons"`
}

// BillingSubscription описывает текущую подписку и лимиты пользователя.
type BillingSubscription struct {
	PlanID           string       `json:"plan_id"`
	PlanName         string       `json:"plan_name"`
	RenewsAt         *time.Time   `json:"renews_at,omitempty"`
	CancelsAt        *time.Time   `json:"cancels_at,omitempty"`
	HasPaymentMethod bool         `json:"has_payment_method"`
	Usage            BillingUsage `json:"usage"`
}

// BillingUsage описывает использование месячного лимита.
type BillingUsage struct {
	Value int `json:"value"`
	Max   int `json:"max"`
}

// BillingPlan описывает один доступный тариф.
type BillingPlan struct {
	ID                 string               `json:"id"`
	Name               string               `json:"name"`
	MonthlyPrice       int                  `json:"monthly_price"`
	YearlyMonthlyPrice int                  `json:"yearly_monthly_price,omitempty"`
	CardsPerMonth      int                  `json:"cards_per_month"`
	Features           []BillingPlanFeature `json:"features"`
	Current            bool                 `json:"current,omitempty"`
	Popular            bool                 `json:"popular,omitempty"`
}

// BillingPlanFeature описывает одну возможность тарифа.
type BillingPlanFeature struct {
	Label   string `json:"label"`
	Enabled bool   `json:"enabled"`
}

// BillingAddon описывает разовый пакет карточек.
type BillingAddon struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Price       int    `json:"price"`
}

// CreateCheckoutRequest описывает запрос на оплату тарифа.
type CreateCheckoutRequest struct {
	PlanID string            `json:"plan_id"`
	Period BillingPlanPeriod `json:"period"`
}

// BillingPlanPeriod описывает период оплаты тарифа.
type BillingPlanPeriod string

const (
	// BillingPlanPeriodMonthly описывает оплату тарифа на один месяц.
	BillingPlanPeriodMonthly BillingPlanPeriod = "monthly"
	// BillingPlanPeriodYearly описывает оплату тарифа на один год.
	BillingPlanPeriodYearly BillingPlanPeriod = "yearly"
)

// CreateCheckoutResponse возвращается после создания checkout-сессии.
type CreateCheckoutResponse struct {
	CheckoutURL string `json:"checkout_url"`
}

// PurchaseAddonRequest описывает покупку разового пакета карточек.
type PurchaseAddonRequest struct {
	AddonID string `json:"addon_id"`
}

// PurchaseAddonResponse подтверждает, что checkout для пакета создан.
type PurchaseAddonResponse struct {
	CheckoutURL string `json:"checkout_url"`
}

// CancelSubscriptionResponse подтверждает отмену автопродления подписки.
type CancelSubscriptionResponse struct {
	Status string `json:"status"`
}
