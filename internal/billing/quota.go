package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kartochki-online-backend/internal/dbgen"
)

// EnsureGenerationAllowed проверяет, что новый запуск генерации помещается в текущий billing-лимит.
func (s *Service) EnsureGenerationAllowed(ctx context.Context, userID string, requestedCards int) error {
	uid, err := parseUserID(userID)
	if err != nil {
		return ErrUserNotFound
	}
	if requestedCards <= 0 {
		return GenerationLimitExceededError{RequestedCards: requestedCards}
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
		// Передаём детали лимита наверх, чтобы клиент получил не сухой код ошибки,
		// а понятное объяснение с числами текущего периода.
		return GenerationLimitExceededError{
			CardsLimit:     int(usageQuota.CardsLimit),
			ReservedCards:  int(reservedCards),
			RequestedCards: requestedCards,
			PeriodEnd:      usageQuota.PeriodEnd.Time,
		}
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
