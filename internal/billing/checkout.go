package billing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kartochki-online-backend/internal/dbgen"
)

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

	targetPlan, err := s.queries.GetBillingPlanByCode(ctx, strings.TrimSpace(input.PlanID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CheckoutResult{}, ErrPlanNotFound
		}
		return CheckoutResult{}, fmt.Errorf("%w: get billing plan by code: %v", ErrCheckoutPreparationFailed, err)
	}

	currentSubscription, _, err := s.getCurrentBillingSnapshot(ctx, uid)
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("%w: get current billing snapshot: %v", ErrCheckoutPreparationFailed, err)
	}
	if strings.TrimSpace(currentSubscription.PlanCode) == targetPlan.Code {
		return CheckoutResult{}, ErrPlanAlreadyActive
	}

	amount, err := amountForPeriod(targetPlan, period)
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("%w: resolve amount for period: %v", ErrCheckoutPreparationFailed, err)
	}

	customerEmail, err := s.checkoutCustomerEmail(ctx, uid, input.CustomerEmail)
	if err != nil {
		return CheckoutResult{}, err
	}

	// Ключ уникален для каждой checkout-попытки, чтобы отменённый платёж не блокировал новый.
	idempotencyKey := checkoutIdempotencyKey(uid.String(), targetPlan.Code, string(period), uuid.NewString())
	session, err := s.provider.CreateSubscriptionCheckout(ctx, SubscriptionCheckoutInput{
		UserID:             uid.String(),
		CustomerEmail:      customerEmail,
		PlanCode:           targetPlan.Code,
		Period:             period,
		Amount:             amount,
		Currency:           checkoutCurrencyRUB,
		ReceiptDescription: strings.TrimSpace(targetPlan.Name),
		IdempotencyKey:     idempotencyKey,
	})
	if err != nil {
		return CheckoutResult{}, fmt.Errorf("%w: %v", ErrCheckoutProviderFailed, err)
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
		return CheckoutResult{}, fmt.Errorf("%w: %v", ErrCheckoutPersistenceFailed, err)
	}

	return CheckoutResult{CheckoutURL: session.CheckoutURL}, nil
}

// PurchaseAddon валидирует покупку разового пакета и делегирует checkout платёжному провайдеру.
func (s *Service) PurchaseAddon(ctx context.Context, input PurchaseAddonInput) (PurchaseAddonResult, error) {
	uid, err := parseUserID(input.UserID)
	if err != nil {
		return PurchaseAddonResult{}, ErrUserNotFound
	}

	addon, err := s.queries.GetAddonProductByCode(ctx, strings.TrimSpace(input.AddonID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PurchaseAddonResult{}, ErrAddonNotFound
		}
		return PurchaseAddonResult{}, fmt.Errorf("%w: get billing addon by code: %v", ErrCheckoutPreparationFailed, err)
	}

	customerEmail, err := s.checkoutCustomerEmail(ctx, uid, input.CustomerEmail)
	if err != nil {
		return PurchaseAddonResult{}, err
	}

	// Для addon также создаём отдельную checkout-попытку на каждый клик оплаты.
	idempotencyKey := checkoutIdempotencyKey(uid.String(), addon.Code, uuid.NewString())
	session, err := s.provider.CreateAddonCheckout(ctx, AddonCheckoutInput{
		UserID:             uid.String(),
		CustomerEmail:      customerEmail,
		AddonCode:          addon.Code,
		Amount:             int(addon.Price),
		Currency:           checkoutCurrencyRUB,
		ReceiptDescription: strings.TrimSpace(addon.Title),
		IdempotencyKey:     idempotencyKey,
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
		return PurchaseAddonResult{}, fmt.Errorf("%w: %v", ErrCheckoutPersistenceFailed, err)
	}

	return PurchaseAddonResult{CheckoutURL: session.CheckoutURL}, nil
}

func (s *Service) checkoutCustomerEmail(ctx context.Context, userID uuid.UUID, requestedEmail string) (string, error) {
	user, err := s.queries.GetAuthUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrUserNotFound
		}
		return "", fmt.Errorf("%w: get billing user: %v", ErrCheckoutPreparationFailed, err)
	}

	email := strings.TrimSpace(user.Email)
	if email == "" {
		email = normalizeCheckoutEmail(requestedEmail)
		if email == "" {
			// ЮКасса требует email покупателя для фискального чека при обычном checkout.
			return "", ErrCheckoutEmailRequired
		}
		if !isValidCheckoutEmail(email) {
			return "", ErrCheckoutEmailInvalid
		}

		// Email из checkout сохраняем в профиль только если у пользователя его ещё нет.
		// Подтверждение email здесь не ставим: адрес нужен для чека, а не для входа или сброса пароля.
		if _, err := s.queries.AssignUserEmailIfMissing(ctx, dbgen.AssignUserEmailIfMissingParams{
			ID:    userID,
			Email: toPgText(email),
		}); err != nil {
			if isUniqueViolation(err) {
				return "", ErrCheckoutEmailTaken
			}
			if errors.Is(err, pgx.ErrNoRows) {
				return s.checkoutCustomerEmail(ctx, userID, "")
			}
			return "", fmt.Errorf("%w: save checkout email: %v", ErrCheckoutPreparationFailed, err)
		}
	}

	return email, nil
}

func normalizeCheckoutEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func isValidCheckoutEmail(email string) bool {
	parsed, err := mail.ParseAddress(email)
	return err == nil && parsed.Address == email
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

func (noopCheckoutProvider) CreateSubscriptionCheckout(context.Context, SubscriptionCheckoutInput) (CheckoutSession, error) {
	return CheckoutSession{}, ErrCheckoutProviderNotConfigured
}

func (noopCheckoutProvider) CreateAddonCheckout(context.Context, AddonCheckoutInput) (CheckoutSession, error) {
	return CheckoutSession{}, ErrCheckoutProviderNotConfigured
}
