package billing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kartochki-online-backend/internal/dbgen"
)

const (
	freePlanCode        = "free"
	checkoutCurrencyRUB = "RUB"
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
	queries  *dbgen.Queries
	provider checkoutProvider
}

type checkoutProvider interface {
	CreateSubscriptionCheckout(ctx context.Context, input subscriptionCheckoutInput) (string, error)
	CreateAddonCheckout(ctx context.Context, input addonCheckoutInput) (string, error)
}

type subscriptionCheckoutInput struct {
	UserID   string
	PlanCode string
	Period   PlanPeriod
	Amount   int
	Currency string
}

type addonCheckoutInput struct {
	UserID    string
	AddonCode string
	Amount    int
	Currency  string
}

type noopCheckoutProvider struct{}

// NewService создаёт billing-сервис.
func NewService(queries *dbgen.Queries) *Service {
	return &Service{
		queries:  queries,
		provider: noopCheckoutProvider{},
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
		return CheckoutResult{}, err
	}
	if strings.TrimSpace(currentSubscription.PlanCode) == targetPlan.Code {
		return CheckoutResult{}, ErrPlanAlreadyActive
	}

	amount, err := amountForPeriod(targetPlan, period)
	if err != nil {
		return CheckoutResult{}, err
	}

	checkoutURL, err := s.provider.CreateSubscriptionCheckout(ctx, subscriptionCheckoutInput{
		UserID:   uid.String(),
		PlanCode: targetPlan.Code,
		Period:   period,
		Amount:   amount,
		Currency: checkoutCurrencyRUB,
	})
	if err != nil {
		return CheckoutResult{}, err
	}

	return CheckoutResult{CheckoutURL: checkoutURL}, nil
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

	checkoutURL, err := s.provider.CreateAddonCheckout(ctx, addonCheckoutInput{
		UserID:    uid.String(),
		AddonCode: addon.Code,
		Amount:    int(addon.Price),
		Currency:  checkoutCurrencyRUB,
	})
	if err != nil {
		return PurchaseAddonResult{}, err
	}

	return PurchaseAddonResult{CheckoutURL: checkoutURL}, nil
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
	if row.Status == "scheduled_cancel" {
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
			Status:             "active",
			Provider:           "manual",
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

func toPlans(rows []dbgen.Plan, currentPlanCode string) []Plan {
	result := make([]Plan, len(rows))
	for i, row := range rows {
		result[i] = Plan{
			ID:                 row.Code,
			Name:               strings.TrimSpace(row.Name),
			MonthlyPrice:       int(row.MonthlyPrice),
			YearlyMonthlyPrice: int32Value(row.YearlyMonthlyPrice),
			CardsPerMonth:      int(row.CardsPerMonth),
			Features:           clonePlanFeatures(planFeaturesByCode[row.Code]),
			Current:            row.Code == currentPlanCode,
			Popular:            row.IsPopular,
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

func parseUserID(userID string) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(userID))
}

func currentMonthPeriod(now time.Time) (time.Time, time.Time) {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0)
}

func toTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func nullableTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}

	result := value.Time
	return &result
}

func int32Value(value pgtype.Int4) int {
	if !value.Valid {
		return 0
	}

	return int(value.Int32)
}

func (noopCheckoutProvider) CreateSubscriptionCheckout(context.Context, subscriptionCheckoutInput) (string, error) {
	return "", ErrCheckoutProviderNotConfigured
}

func (noopCheckoutProvider) CreateAddonCheckout(context.Context, addonCheckoutInput) (string, error) {
	return "", ErrCheckoutProviderNotConfigured
}
