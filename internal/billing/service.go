package billing

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"kartochki-online-backend/internal/dbgen"
)

const (
	freePlanCode        = "free"
	checkoutCurrencyRUB = "RUB"

	// ProviderYooKassa идентифицирует ЮКасса в колонке provider таблицы subscriptions.
	ProviderYooKassa = "yookassa"
	// providerManual используется для виртуальных подписок (бесплатный план без внешнего провайдера).
	providerManual = "manual"

	subscriptionStatusActive          = "active"
	subscriptionStatusScheduledCancel = "scheduled_cancel"

	paymentStatusPending = "pending"

	paymentTypeSubscription        = "subscription"
	paymentTypeSubscriptionRenewal = "subscription_renewal"
	paymentTypeAddon               = "addon"
)

// NewService создаёт billing-сервис.
// Если provider равен nil — используется noopCheckoutProvider (checkout недоступен, но остальные операции работают).
// pool и queries обязательны: nil вызовет панику при первом обращении к БД.
func NewService(pool *pgxpool.Pool, queries *dbgen.Queries, provider CheckoutProvider) *Service {
	if pool == nil {
		panic("billing.NewService: pool is nil")
	}
	if queries == nil {
		panic("billing.NewService: queries is nil")
	}
	if provider == nil {
		provider = noopCheckoutProvider{}
	}
	return &Service{
		pool:     pool,
		queries:  queries,
		provider: provider,
	}
}
