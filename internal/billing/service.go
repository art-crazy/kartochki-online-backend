package billing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

var planFeaturesByCode = map[string][]PlanFeature{
	"free": {
		{Label: "До 30 карточек в месяц", Enabled: true},
		{Label: "Базовые стили генерации", Enabled: true},
		{Label: "Экспорт архивом", Enabled: false},
	},
	"pro": {
		{Label: "До 500 карточек в месяц", Enabled: true},
		{Label: "Все стили генерации", Enabled: true},
		{Label: "Экспорт архивом", Enabled: true},
	},
	"business": {
		{Label: "До 2500 карточек в месяц", Enabled: true},
		{Label: "Командный запас по лимитам", Enabled: true},
		{Label: "Приоритет на будущие интеграции", Enabled: true},
	},
}

type planCatalogItem struct {
	Name               string
	MonthlyPrice       int
	YearlyMonthlyPrice int
	CardsPerMonth      int
	Popular            bool
	Features           []PlanFeature
}

var planCatalogByCode = map[string]planCatalogItem{
	"free": {
		Name:          "Старт",
		MonthlyPrice:  0,
		CardsPerMonth: 5,
		Features: []PlanFeature{
			{Label: "Для теста сервиса и первых карточек товара", Enabled: true},
			{Label: "До 5 карточек в месяц", Enabled: true},
			{Label: "Генерация инфографики, фото и текстов", Enabled: true},
			{Label: "Подходит для знакомства, но не для постоянного потока", Enabled: false},
		},
	},
	"business": {
		Name:               "Бизнес",
		MonthlyPrice:       1490,
		YearlyMonthlyPrice: 1192,
		CardsPerMonth:      75,
		Popular:            true,
		Features: []PlanFeature{
			{Label: "Для регулярной работы селлера с каталогом", Enabled: true},
			{Label: "До 75 карточек в месяц", Enabled: true},
			{Label: "Полный доступ к генерации изображений и текстов", Enabled: true},
			{Label: "Без интеграции по API", Enabled: false},
		},
	},
	"agency": {
		Name:               "Агентство",
		MonthlyPrice:       4990,
		YearlyMonthlyPrice: 3992,
		CardsPerMonth:      250,
		Features: []PlanFeature{
			{Label: "Для агентств и команд с несколькими проектами", Enabled: true},
			{Label: "До 250 карточек в месяц", Enabled: true},
			{Label: "Полный доступ ко всем функциям сервиса", Enabled: true},
			{Label: "Приоритетная поддержка", Enabled: true},
			{Label: "Без интеграции по API", Enabled: false},
		},
	},
	"corporate": {
		Name:               "Корпоративный",
		MonthlyPrice:       14990,
		YearlyMonthlyPrice: 11992,
		CardsPerMonth:      750,
		Features: []PlanFeature{
			{Label: "Для крупных команд и потоковой генерации карточек", Enabled: true},
			{Label: "До 750 карточек в месяц", Enabled: true},
			{Label: "Полный доступ ко всем функциям сервиса", Enabled: true},
			{Label: "Приоритетная поддержка", Enabled: true},
			{Label: "Интеграция по API", Enabled: true},
		},
	},
}

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
	Usage            Usage
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
	UserID   string
	PlanCode string
	Period   PlanPeriod
	Amount   int
	Currency string
	// IdempotencyKey — стабильный ключ для дедупликации запроса на стороне платёжного провайдера.
	// Вычисляется в billing-сервисе, чтобы провайдер не создавал дублирующий платёж при повторных попытках.
	IdempotencyKey string
}

// AddonCheckoutInput описывает параметры checkout для покупки разового пакета.
type AddonCheckoutInput struct {
	UserID    string
	AddonCode string
	Amount    int
	Currency  string
	// IdempotencyKey — стабильный ключ для дедупликации запроса на стороне платёжного провайдера.
	IdempotencyKey string
}

// RecurringPaymentInput описывает параметры автосписания за продление подписки.
type RecurringPaymentInput struct {
	UserID          string
	PlanCode        string
	Period          PlanPeriod
	Amount          int
	Currency        string
	PaymentMethodID string
	IdempotencyKey  string
}

type noopCheckoutProvider struct{}

var _ CheckoutProvider = noopCheckoutProvider{}

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

// Get собирает текущее billing-состояние пользователя.
func (s *Service) Get(ctx context.Context, userID string) (Billing, error) {
	uid, err := parseUserID(userID)
	if err != nil {
		return Billing{}, ErrUserNotFound
	}

	if err := s.ensureUserExists(ctx, uid); err != nil {
		return Billing{}, err
	}

	subscription, usageQuota, err := s.getCurrentBillingSnapshot(ctx, uid)
	if err != nil {
		return Billing{}, err
	}

	usedCards, err := s.queries.CountGeneratedCardsForUserInPeriod(ctx, dbgen.CountGeneratedCardsForUserInPeriodParams{
		UserID:      uid,
		PeriodStart: usageQuota.PeriodStart,
		PeriodEnd:   usageQuota.PeriodEnd,
	})
	if err != nil {
		return Billing{}, fmt.Errorf("count generated cards for billing usage: %w", err)
	}

	planRows, err := s.queries.ListActiveBillingPlans(ctx)
	if err != nil {
		return Billing{}, fmt.Errorf("list billing plans: %w", err)
	}

	addonRows, err := s.queries.ListActiveAddonProducts(ctx)
	if err != nil {
		return Billing{}, fmt.Errorf("list billing addons: %w", err)
	}

	currentPlanCode := strings.TrimSpace(subscription.PlanCode)
	return Billing{
		CurrentSubscription: Subscription{
			PlanID:           currentPlanCode,
			PlanName:         strings.TrimSpace(subscription.PlanName),
			RenewsAt:         nullableTime(subscription.RenewsAt),
			CancelsAt:        nullableTime(subscription.CancelsAt),
			HasPaymentMethod: subscription.HasPaymentMethod,
			Usage: Usage{
				Value: int(usedCards),
				Max:   int(usageQuota.CardsLimit),
			},
		},
		Plans:  toPlans(planRows, currentPlanCode),
		Addons: toAddons(addonRows),
	}, nil
}

// CreateCheckout валидирует сценарий покупки тарифа и делегирует создание checkout платёжному провайдеру.
func (s *Service) CreateCheckout(ctx context.Context, input CheckoutInput) (CheckoutResult, error) {
	uid, err := parseUserID(input.UserID)
	if err != nil {
		return CheckoutResult{}, ErrUserNotFound
	}

	period := PlanPeriod(strings.TrimSpace(string(input.Period)))
	if period != PlanPeriodMonthly && period != PlanPeriodYearly {
		return CheckoutResult{}, ErrInvalidPlanPeriod
	}

	if err := s.ensureUserExists(ctx, uid); err != nil {
		return CheckoutResult{}, err
	}

	targetPlan, err := s.queries.GetBillingPlanByCode(ctx, strings.TrimSpace(input.PlanID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CheckoutResult{}, ErrPlanNotFound
		}
		return CheckoutResult{}, fmt.Errorf("get billing plan by code: %w", err)
	}

	currentSubscription, _, err := s.getCurrentBillingSnapshot(ctx, uid)
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("%w: %v", ErrCheckoutProviderFailed, err)
	}
	if strings.TrimSpace(currentSubscription.PlanCode) == targetPlan.Code {
		return CheckoutResult{}, ErrPlanAlreadyActive
	}

	amount, err := amountForPeriod(targetPlan, period)
	if err != nil {
		return CheckoutResult{}, err
	}

	// Ключ уникален для каждой checkout-попытки, чтобы отменённый платёж не блокировал новый.
	idempotencyKey := checkoutIdempotencyKey(uid.String(), targetPlan.Code, string(period), uuid.NewString())
	session, err := s.provider.CreateSubscriptionCheckout(ctx, SubscriptionCheckoutInput{
		UserID:         uid.String(),
		PlanCode:       targetPlan.Code,
		Period:         period,
		Amount:         amount,
		Currency:       checkoutCurrencyRUB,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return CheckoutResult{}, err
	}

	if err := s.recordPendingPayment(ctx, dbgen.CreatePaymentParams{
		UserID:            uid,
		SubscriptionID:    pgtype.UUID{},
		AddonProductID:    pgtype.UUID{},
		Provider:          ProviderYooKassa,
		ProviderPaymentID: toPgText(session.ProviderPaymentID),
		Kind:              paymentTypeSubscription,
		Status:            paymentStatusPending,
		Amount:            int32(amount),
		Currency:          checkoutCurrencyRUB,
		CheckoutUrl:       toPgText(session.CheckoutURL),
	}); err != nil {
		return CheckoutResult{}, err
	}

	return CheckoutResult{CheckoutURL: session.CheckoutURL}, nil
}

// PurchaseAddon валидирует покупку разового пакета и делегирует checkout платёжному провайдеру.
func (s *Service) PurchaseAddon(ctx context.Context, input PurchaseAddonInput) (PurchaseAddonResult, error) {
	uid, err := parseUserID(input.UserID)
	if err != nil {
		return PurchaseAddonResult{}, ErrUserNotFound
	}

	if err := s.ensureUserExists(ctx, uid); err != nil {
		return PurchaseAddonResult{}, err
	}

	addon, err := s.queries.GetAddonProductByCode(ctx, strings.TrimSpace(input.AddonID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PurchaseAddonResult{}, ErrAddonNotFound
		}
		return PurchaseAddonResult{}, fmt.Errorf("get billing addon by code: %w", err)
	}

	// Для addon также создаём отдельную checkout-попытку на каждый клик оплаты.
	idempotencyKey := checkoutIdempotencyKey(uid.String(), addon.Code, uuid.NewString())
	session, err := s.provider.CreateAddonCheckout(ctx, AddonCheckoutInput{
		UserID:         uid.String(),
		AddonCode:      addon.Code,
		Amount:         int(addon.Price),
		Currency:       checkoutCurrencyRUB,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return PurchaseAddonResult{}, fmt.Errorf("%w: %v", ErrCheckoutProviderFailed, err)
	}

	if err := s.recordPendingPayment(ctx, dbgen.CreatePaymentParams{
		UserID:            uid,
		SubscriptionID:    pgtype.UUID{},
		AddonProductID:    toPgUUID(addon.ID),
		Provider:          ProviderYooKassa,
		ProviderPaymentID: toPgText(session.ProviderPaymentID),
		Kind:              paymentTypeAddon,
		Status:            paymentStatusPending,
		Amount:            int32(addon.Price),
		Currency:          checkoutCurrencyRUB,
		CheckoutUrl:       toPgText(session.CheckoutURL),
	}); err != nil {
		return PurchaseAddonResult{}, err
	}

	return PurchaseAddonResult{CheckoutURL: session.CheckoutURL}, nil
}

// RenewDueSubscriptions создаёт рекуррентные платежи для подписок, у которых наступила дата renews_at.
// Фактическое продление периода выполняется позже через webhook payment.succeeded.
func (s *Service) RenewDueSubscriptions(ctx context.Context, batchLimit int) (int, error) {
	if batchLimit <= 0 {
		batchLimit = 50
	}

	rows, err := s.queries.ListSubscriptionsDueForRenewal(ctx, dbgen.ListSubscriptionsDueForRenewalParams{
		NowAt:      toTimestamp(time.Now().UTC()),
		BatchLimit: int32(batchLimit),
	})
	if err != nil {
		return 0, fmt.Errorf("list subscriptions due for renewal: %w", err)
	}

	created := 0
	var joined error
	for _, row := range rows {
		if err := s.createRenewalPayment(ctx, row); err != nil {
			joined = errors.Join(joined, err)
			continue
		}
		created++
	}

	return created, joined
}

func (s *Service) createRenewalPayment(ctx context.Context, row dbgen.ListSubscriptionsDueForRenewalRow) error {
	period := renewalPeriod(row.CurrentPeriodStart.Time, row.CurrentPeriodEnd.Time)
	amount, err := amountForRenewal(row, period)
	if err != nil {
		return err
	}

	session, err := s.provider.CreateRecurringPayment(ctx, RecurringPaymentInput{
		UserID:          row.UserID.String(),
		PlanCode:        row.PlanCode,
		Period:          period,
		Amount:          amount,
		Currency:        checkoutCurrencyRUB,
		PaymentMethodID: row.ProviderSubscriptionID.String,
		IdempotencyKey:  checkoutIdempotencyKey(row.ID.String(), row.CurrentPeriodEnd.Time.UTC().Format(time.RFC3339), string(period)),
	})
	if err != nil {
		return fmt.Errorf("%w: create recurring payment for subscription %s: %v", ErrCheckoutProviderFailed, row.ID, err)
	}

	if err := s.recordPendingPayment(ctx, dbgen.CreatePaymentParams{
		UserID:            row.UserID,
		SubscriptionID:    toPgUUID(row.ID),
		AddonProductID:    pgtype.UUID{},
		Provider:          ProviderYooKassa,
		ProviderPaymentID: toPgText(session.ProviderPaymentID),
		Kind:              paymentTypeSubscription,
		Status:            paymentStatusPending,
		Amount:            int32(amount),
		Currency:          checkoutCurrencyRUB,
		CheckoutUrl:       pgtype.Text{},
	}); err != nil {
		return err
	}

	return nil
}

// CancelSubscription ставит активную платную подписку на отмену в конце текущего периода.
func (s *Service) CancelSubscription(ctx context.Context, userID string) error {
	uid, err := parseUserID(userID)
	if err != nil {
		return ErrUserNotFound
	}

	row, err := s.queries.GetCurrentSubscriptionByUserID(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSubscriptionNotCancelable
		}
		return fmt.Errorf("get current subscription before cancel: %w", err)
	}

	if strings.TrimSpace(row.PlanCode) == freePlanCode {
		return ErrSubscriptionNotCancelable
	}
	if row.Status == subscriptionStatusScheduledCancel {
		return nil
	}

	affected, err := s.queries.MarkSubscriptionScheduledCancel(ctx, dbgen.MarkSubscriptionScheduledCancelParams{
		ID:        row.ID,
		UserID:    uid,
		CancelsAt: row.CurrentPeriodEnd,
	})
	if err != nil {
		return fmt.Errorf("mark subscription scheduled cancel: %w", err)
	}
	if affected == 0 {
		return ErrSubscriptionNotCancelable
	}

	return nil
}

// EnsureGenerationAllowed проверяет, что новый запуск генерации помещается в текущий billing-лимит.
func (s *Service) EnsureGenerationAllowed(ctx context.Context, userID string, requestedCards int) error {
	uid, err := parseUserID(userID)
	if err != nil {
		return ErrUserNotFound
	}
	if requestedCards <= 0 {
		return ErrGenerationLimitExceeded
	}

	if err := s.ensureUserExists(ctx, uid); err != nil {
		return err
	}

	_, usageQuota, err := s.getCurrentBillingSnapshot(ctx, uid)
	if err != nil {
		return err
	}

	reservedCards, err := s.queries.SumReservedGenerationCardsForUserInPeriod(ctx, dbgen.SumReservedGenerationCardsForUserInPeriodParams{
		UserID:      uid,
		PeriodStart: usageQuota.PeriodStart,
		PeriodEnd:   usageQuota.PeriodEnd,
	})
	if err != nil {
		return fmt.Errorf("sum reserved generation cards for billing limit: %w", err)
	}
	if int(reservedCards)+requestedCards > int(usageQuota.CardsLimit) {
		return ErrGenerationLimitExceeded
	}

	return nil
}

// getCurrentBillingSnapshot собирает billing-срез для read-only сценария.
// Метод не создаёт новые записи, чтобы обычный GET /billing не менял состояние БД.
func (s *Service) getCurrentBillingSnapshot(ctx context.Context, userID uuid.UUID) (dbgen.GetCurrentSubscriptionByUserIDRow, dbgen.UsageQuota, error) {
	subscription, err := s.queries.GetCurrentSubscriptionByUserID(ctx, userID)
	if err == nil {
		quota, quotaErr := s.getUsageQuotaSnapshot(ctx, subscription)
		if quotaErr != nil {
			return dbgen.GetCurrentSubscriptionByUserIDRow{}, dbgen.UsageQuota{}, quotaErr
		}
		return subscription, quota, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return dbgen.GetCurrentSubscriptionByUserIDRow{}, dbgen.UsageQuota{}, fmt.Errorf("get current subscription: %w", err)
	}

	return s.buildFreeBillingSnapshot(ctx, userID)
}

// getUsageQuotaSnapshot читает текущую квоту без автосоздания строки usage_quotas.
// Это важно для экранов чтения: отсутствие квоты не должно само по себе порождать запись в БД.
func (s *Service) getUsageQuotaSnapshot(ctx context.Context, subscription dbgen.GetCurrentSubscriptionByUserIDRow) (dbgen.UsageQuota, error) {
	now := time.Now().UTC()
	row, err := s.queries.GetCurrentUsageQuotaBySubscriptionID(ctx, dbgen.GetCurrentUsageQuotaBySubscriptionIDParams{
		SubscriptionID: subscription.ID,
		NowAt:          toTimestamp(now),
	})
	if err == nil {
		return row, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return dbgen.UsageQuota{}, fmt.Errorf("get current usage quota: %w", err)
	}

	periodStart := subscription.CurrentPeriodStart
	periodEnd := subscription.CurrentPeriodEnd
	if !periodStart.Valid || !periodEnd.Valid || !periodStart.Time.Before(periodEnd.Time) {
		fallbackStart, fallbackEnd := currentMonthPeriod(now)
		periodStart = toTimestamp(fallbackStart)
		periodEnd = toTimestamp(fallbackEnd)
	}

	return dbgen.UsageQuota{
		UserID:         subscription.UserID,
		SubscriptionID: subscription.ID,
		PeriodStart:    periodStart,
		PeriodEnd:      periodEnd,
		CardsLimit:     subscription.CardsPerMonth,
		CardsUsed:      0,
	}, nil
}

// buildFreeBillingSnapshot возвращает виртуальную бесплатную подписку для новых пользователей.
// Так billing-экран сразу консистентен, но данные в subscriptions создаются только когда это реально нужно бизнес-логике.
func (s *Service) buildFreeBillingSnapshot(ctx context.Context, userID uuid.UUID) (dbgen.GetCurrentSubscriptionByUserIDRow, dbgen.UsageQuota, error) {
	plan, err := s.queries.GetBillingPlanByCode(ctx, freePlanCode)
	if err != nil {
		// Если план не найден — миграция не применена. Возвращаем явную ошибку,
		// чтобы оператор сразу понял причину, а не получил невнятный internal_error.
		if errors.Is(err, pgx.ErrNoRows) {
			return dbgen.GetCurrentSubscriptionByUserIDRow{}, dbgen.UsageQuota{}, ErrFreePlanNotFound
		}
		return dbgen.GetCurrentSubscriptionByUserIDRow{}, dbgen.UsageQuota{}, fmt.Errorf("get free billing plan: %w", err)
	}

	periodStart, periodEnd := currentMonthPeriod(time.Now().UTC())
	return dbgen.GetCurrentSubscriptionByUserIDRow{
			UserID:             userID,
			PlanID:             plan.ID,
			Status:             subscriptionStatusActive,
			Provider:           providerManual,
			HasPaymentMethod:   false,
			StartedAt:          toTimestamp(periodStart),
			CurrentPeriodStart: toTimestamp(periodStart),
			CurrentPeriodEnd:   toTimestamp(periodEnd),
			RenewsAt:           toTimestamp(periodEnd),
			PlanCode:           plan.Code,
			PlanName:           plan.Name,
			CardsPerMonth:      plan.CardsPerMonth,
		}, dbgen.UsageQuota{
			UserID:      userID,
			PeriodStart: toTimestamp(periodStart),
			PeriodEnd:   toTimestamp(periodEnd),
			CardsLimit:  plan.CardsPerMonth,
			CardsUsed:   0,
		}, nil
}

func (s *Service) ensureUserExists(ctx context.Context, userID uuid.UUID) error {
	if _, err := s.queries.GetAuthUserByID(ctx, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		return fmt.Errorf("get billing user: %w", err)
	}

	return nil
}

// recordPendingPayment сохраняет созданный checkout в БД.
// Если ЮКасса вернула тот же payment_id по ключу идемпотентности, повторный запрос безопасно переиспользует запись.
func (s *Service) recordPendingPayment(ctx context.Context, params dbgen.CreatePaymentParams) error {
	if _, err := s.queries.GetPaymentByProviderID(ctx, params.ProviderPaymentID); err == nil {
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("get pending payment before create: %w", err)
	}

	if _, err := s.queries.CreatePayment(ctx, params); err != nil {
		return fmt.Errorf("create pending payment: %w", err)
	}

	return nil
}

func toPlans(rows []dbgen.Plan, currentPlanCode string) []Plan {
	result := make([]Plan, len(rows))
	for i, row := range rows {
		catalog, ok := planCatalogByCode[row.Code]
		name := strings.TrimSpace(row.Name)
		monthlyPrice := int(row.MonthlyPrice)
		yearlyMonthlyPrice := int32Value(row.YearlyMonthlyPrice)
		cardsPerMonth := int(row.CardsPerMonth)
		features := clonePlanFeatures(planFeaturesByCode[row.Code])
		popular := row.IsPopular

		if ok {
			name = catalog.Name
			monthlyPrice = catalog.MonthlyPrice
			yearlyMonthlyPrice = catalog.YearlyMonthlyPrice
			cardsPerMonth = catalog.CardsPerMonth
			features = clonePlanFeatures(catalog.Features)
			popular = catalog.Popular
		}

		result[i] = Plan{
			ID:                 row.Code,
			Name:               name,
			MonthlyPrice:       monthlyPrice,
			YearlyMonthlyPrice: yearlyMonthlyPrice,
			CardsPerMonth:      cardsPerMonth,
			Features:           features,
			Current:            row.Code == currentPlanCode,
			Popular:            popular,
		}
	}

	return result
}

func toAddons(rows []dbgen.AddonProduct) []Addon {
	result := make([]Addon, len(rows))
	for i, row := range rows {
		result[i] = Addon{
			ID:          row.Code,
			Title:       strings.TrimSpace(row.Title),
			Description: strings.TrimSpace(row.Description),
			Price:       int(row.Price),
		}
	}

	return result
}

func clonePlanFeatures(features []PlanFeature) []PlanFeature {
	if len(features) == 0 {
		return nil
	}

	result := make([]PlanFeature, len(features))
	copy(result, features)
	return result
}

// checkoutIdempotencyKey возвращает ключ для дедупликации одного запроса к платёжному провайдеру.
// Последняя часть обычно случайная, чтобы новая попытка оплаты не переиспользовала старый отменённый платёж.
func checkoutIdempotencyKey(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, ":")))
	return hex.EncodeToString(h[:])
}

func amountForPeriod(plan dbgen.Plan, period PlanPeriod) (int, error) {
	switch period {
	case PlanPeriodMonthly:
		return int(plan.MonthlyPrice), nil
	case PlanPeriodYearly:
		if !plan.YearlyMonthlyPrice.Valid {
			return 0, ErrInvalidPlanPeriod
		}
		return int(plan.YearlyMonthlyPrice.Int32) * 12, nil
	default:
		return 0, ErrInvalidPlanPeriod
	}
}

func amountForRenewal(row dbgen.ListSubscriptionsDueForRenewalRow, period PlanPeriod) (int, error) {
	switch period {
	case PlanPeriodMonthly:
		return int(row.MonthlyPrice), nil
	case PlanPeriodYearly:
		if !row.YearlyMonthlyPrice.Valid {
			return 0, ErrInvalidPlanPeriod
		}
		return int(row.YearlyMonthlyPrice.Int32) * 12, nil
	default:
		return 0, ErrInvalidPlanPeriod
	}
}

func renewalPeriod(start, end time.Time) PlanPeriod {
	if start.AddDate(1, 0, 0).Equal(end) {
		return PlanPeriodYearly
	}

	return PlanPeriodMonthly
}

func (noopCheckoutProvider) CreateSubscriptionCheckout(context.Context, SubscriptionCheckoutInput) (CheckoutSession, error) {
	return CheckoutSession{}, ErrCheckoutProviderNotConfigured
}

func (noopCheckoutProvider) CreateAddonCheckout(context.Context, AddonCheckoutInput) (CheckoutSession, error) {
	return CheckoutSession{}, ErrCheckoutProviderNotConfigured
}

func (noopCheckoutProvider) CreateRecurringPayment(context.Context, RecurringPaymentInput) (CheckoutSession, error) {
	return CheckoutSession{}, ErrCheckoutProviderNotConfigured
}
