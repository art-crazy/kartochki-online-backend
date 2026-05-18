package billing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kartochki-online-backend/internal/dbgen"
)

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
		return fmt.Errorf("%w: %v", ErrCheckoutPersistenceFailed, err)
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

func (noopCheckoutProvider) CreateRecurringPayment(context.Context, RecurringPaymentInput) (CheckoutSession, error) {
	return CheckoutSession{}, ErrCheckoutProviderNotConfigured
}
