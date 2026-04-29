package billing

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kartochki-online-backend/internal/dbgen"
)

// ensurePendingPaymentForWebhook восстанавливает запись payments, если checkout был создан в ЮКасса,
// но backend не успел сохранить provider_payment_id до ответа пользователю.
func (s *Service) ensurePendingPaymentForWebhook(ctx context.Context, q *dbgen.Queries, event WebhookEvent) error {
	if _, err := q.GetPaymentByProviderID(ctx, toPgText(event.ProviderPaymentID)); err == nil {
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("get payment before webhook activation: %w", err)
	}

	params, err := s.webhookPaymentCreateParams(ctx, q, event)
	if err != nil {
		return err
	}
	if _, err := q.CreatePayment(ctx, params); err != nil {
		return fmt.Errorf("create missing pending payment from webhook: %w", err)
	}

	return nil
}

func (s *Service) webhookPaymentCreateParams(ctx context.Context, q *dbgen.Queries, event WebhookEvent) (dbgen.CreatePaymentParams, error) {
	userID, err := parseUserID(event.Metadata.UserID)
	if err != nil {
		return dbgen.CreatePaymentParams{}, fmt.Errorf("parse user_id from webhook payment metadata: %w", err)
	}

	amount, err := parseRubAmount(event.Amount.Value)
	if err != nil {
		return dbgen.CreatePaymentParams{}, err
	}

	params := dbgen.CreatePaymentParams{
		UserID:            userID,
		SubscriptionID:    pgtype.UUID{},
		AddonProductID:    pgtype.UUID{},
		Provider:          ProviderYooKassa,
		ProviderPaymentID: toPgText(event.ProviderPaymentID),
		Kind:              event.Metadata.Type,
		Status:            paymentStatusPending,
		Amount:            int32(amount),
		Currency:          event.Amount.Currency,
		CheckoutUrl:       pgtype.Text{},
	}

	switch event.Metadata.Type {
	case paymentTypeSubscription:
		plan, err := q.GetBillingPlanByCode(ctx, event.Metadata.PlanCode)
		if err != nil {
			return dbgen.CreatePaymentParams{}, fmt.Errorf("get plan for webhook payment restore: %w", err)
		}
		expectedAmount, err := amountForPeriod(plan, PlanPeriod(event.Metadata.Period))
		if err != nil {
			return dbgen.CreatePaymentParams{}, err
		}
		if amount != expectedAmount {
			return dbgen.CreatePaymentParams{}, fmt.Errorf("webhook payment amount mismatch for plan %q: got %d, want %d", event.Metadata.PlanCode, amount, expectedAmount)
		}
		return params, nil
	case paymentTypeAddon:
		addon, err := q.GetAddonProductByCode(ctx, event.Metadata.AddonCode)
		if err != nil {
			return dbgen.CreatePaymentParams{}, fmt.Errorf("get addon for webhook payment restore: %w", err)
		}
		if amount != int(addon.Price) {
			return dbgen.CreatePaymentParams{}, fmt.Errorf("webhook payment amount mismatch for addon %q: got %d, want %d", event.Metadata.AddonCode, amount, addon.Price)
		}
		params.AddonProductID = toPgUUID(addon.ID)
		return params, nil
	default:
		return dbgen.CreatePaymentParams{}, fmt.Errorf("unsupported payment type %q in webhook metadata", event.Metadata.Type)
	}
}

func parseRubAmount(value string) (int, error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) == 0 || len(parts) > 2 || parts[0] == "" {
		return 0, fmt.Errorf("parse webhook amount %q: invalid format", value)
	}
	if len(parts) == 2 && strings.TrimRight(parts[1], "0") != "" {
		return 0, fmt.Errorf("parse webhook amount %q: fractional rubles are not supported", value)
	}

	amount, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("parse webhook amount %q: %w", value, err)
	}
	if amount < 0 {
		return 0, fmt.Errorf("parse webhook amount %q: negative amount", value)
	}

	return amount, nil
}
