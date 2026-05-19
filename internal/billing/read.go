package billing

import (
	"context"
	"fmt"
	"strings"

	"kartochki-online-backend/internal/dbgen"
)

// toSavedPaymentMethod преобразует поля карты из строки подписки в доменный тип.
// Возвращает nil, если карта не привязана (last4 пустая).
func toSavedPaymentMethod(row dbgen.GetCurrentSubscriptionByUserIDRow) *SavedPaymentMethod {
	if !row.PaymentMethodLast4.Valid || row.PaymentMethodLast4.String == "" {
		return nil
	}
	return &SavedPaymentMethod{
		Type: row.PaymentMethodType.String,
		Card: &SavedCard{
			Last4:       row.PaymentMethodLast4.String,
			CardType:    row.PaymentMethodCardType.String,
			ExpiryMonth: row.PaymentMethodExpiryMonth.String,
			ExpiryYear:  row.PaymentMethodExpiryYear.String,
		},
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
			PaymentMethod:    toSavedPaymentMethod(subscription),
			Usage: Usage{
				Value: int(usedCards),
				Max:   int(usageQuota.CardsLimit),
			},
		},
		Plans:  toPlans(planRows, currentPlanCode),
		Addons: toAddons(addonRows),
	}, nil
}
