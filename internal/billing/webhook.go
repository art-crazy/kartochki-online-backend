package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kartochki-online-backend/internal/dbgen"
)

// WebhookEventType описывает тип события от платёжного провайдера.
type WebhookEventType string

const (
	// WebhookEventPaymentSucceeded — платёж успешно завершён.
	WebhookEventPaymentSucceeded WebhookEventType = "payment.succeeded"
	// WebhookEventPaymentCanceled — платёж отменён или истёк срок ожидания.
	WebhookEventPaymentCanceled WebhookEventType = "payment.canceled"
)

// WebhookPaymentMetadata хранит поля, переданные при создании платежа.
type WebhookPaymentMetadata struct {
	UserID    string
	PlanCode  string
	AddonCode string
	Period    string
	// Type — тип платежа, передаётся в metadata при создании платежа.
	// Возможные значения определяются billing-сервисом при вызове checkout.
	Type string
}

// WebhookPaymentAmount хранит сумму платежа из события провайдера.
type WebhookPaymentAmount struct {
	Value    string
	Currency string
}

// WebhookEvent описывает нормализованное событие от платёжного провайдера,
// которое billing.Service принимает для обработки независимо от источника.
type WebhookEvent struct {
	// ProviderPaymentID — внешний идентификатор платежа в системе провайдера.
	ProviderPaymentID string
	// ProviderPaymentMethodID — сохранённый способ оплаты ЮКасса для будущих автосписаний.
	ProviderPaymentMethodID string
	// EventType — тип события: payment.succeeded или payment.canceled.
	EventType WebhookEventType
	// PaidAt — время фактического списания (заполняется при payment.succeeded).
	PaidAt *time.Time
	// Amount — сумма платежа от провайдера. Используется для восстановления payments при редком сбое записи checkout.
	Amount WebhookPaymentAmount
	// Metadata — параметры, которые мы передали провайдеру при создании платежа.
	Metadata WebhookPaymentMetadata
}

// HandleWebhookEvent обрабатывает событие от платёжного провайдера.
// Метод идемпотентен: повторный вызов с тем же provider_payment_id безопасен.
func (s *Service) HandleWebhookEvent(ctx context.Context, event WebhookEvent) error {
	switch event.EventType {
	case WebhookEventPaymentSucceeded:
		return s.handlePaymentSucceeded(ctx, event)
	case WebhookEventPaymentCanceled:
		return s.handlePaymentCanceled(ctx, event)
	default:
		// Неизвестное событие не является ошибкой — провайдер может добавлять новые типы.
		return nil
	}
}

// handlePaymentSucceeded активирует подписку или зачисляет addon после успешного платежа.
// Идемпотентность обеспечивается внутри транзакции: MarkPaymentPaid содержит условие
// AND status = 'pending', поэтому повторный webhook не пройдёт дальше (affected == 0).
// Это устраняет TOCTOU-гонку между проверкой и фиксацией в двух параллельных вызовах.
func (s *Service) handlePaymentSucceeded(ctx context.Context, event WebhookEvent) error {
	paidAt := toTimestamp(time.Now().UTC())
	if event.PaidAt != nil {
		paidAt = toTimestamp(*event.PaidAt)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin webhook transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := s.queries.WithTx(tx)

	if err := s.ensurePendingPaymentForWebhook(ctx, qtx, event); err != nil {
		return err
	}

	affected, err := qtx.MarkPaymentPaid(ctx, dbgen.MarkPaymentPaidParams{
		ProviderPaymentID: toPgText(event.ProviderPaymentID),
		PaidAt:            paidAt,
	})
	if err != nil {
		return fmt.Errorf("mark payment paid: %w", err)
	}
	// affected == 0 означает, что платёж уже обработан другим webhook-вызовом.
	if affected == 0 {
		return nil
	}

	switch event.Metadata.Type {
	case paymentTypeSubscription:
		if err := s.activateSubscription(ctx, qtx, event, paidAt); err != nil {
			return err
		}
	case paymentTypeSubscriptionRenewal:
		if err := s.activateSubscription(ctx, qtx, event, paidAt); err != nil {
			return err
		}
	case paymentTypeAddon:
		if err := s.activateAddon(ctx, qtx, event, paidAt); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit webhook transaction: %w", err)
	}

	return nil
}

// activateSubscription создаёт или обновляет подписку после оплаты тарифа.
func (s *Service) activateSubscription(ctx context.Context, q *dbgen.Queries, event WebhookEvent, paidAt pgtype.Timestamptz) error {
	userID, err := parseUserID(event.Metadata.UserID)
	if err != nil {
		return fmt.Errorf("parse user_id from webhook metadata: %w", err)
	}

	plan, err := q.GetBillingPlanByCode(ctx, event.Metadata.PlanCode)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("plan %q not found for webhook activation", event.Metadata.PlanCode)
		}
		return fmt.Errorf("get plan for webhook activation: %w", err)
	}

	period := PlanPeriod(event.Metadata.Period)
	periodStart, periodEnd, err := s.subscriptionPeriodForWebhook(ctx, q, userID, event.Metadata.Type, paidAt.Time, period)
	if err != nil {
		return err
	}

	sub, err := q.UpsertActiveSubscription(ctx, dbgen.UpsertActiveSubscriptionParams{
		UserID:                 userID,
		PlanID:                 plan.ID,
		Provider:               ProviderYooKassa,
		ProviderSubscriptionID: toPgText(subscriptionProviderID(event)),
		HasPaymentMethod:       false,
		StartedAt:              paidAt,
		CurrentPeriodStart:     toTimestamp(periodStart),
		CurrentPeriodEnd:       toTimestamp(periodEnd),
		RenewsAt:               pgtype.Timestamptz{},
	})
	if err != nil {
		return fmt.Errorf("upsert active subscription: %w", err)
	}

	if _, err := q.UpsertUsageQuotaForSubscription(ctx, dbgen.UpsertUsageQuotaForSubscriptionParams{
		UserID:         userID,
		SubscriptionID: sub.ID,
		PeriodStart:    toTimestamp(periodStart),
		PeriodEnd:      toTimestamp(periodEnd),
		CardsLimit:     plan.CardsPerMonth,
	}); err != nil {
		return fmt.Errorf("upsert usage quota after subscription activation: %w", err)
	}

	return nil
}

func subscriptionProviderID(event WebhookEvent) string {
	if event.ProviderPaymentMethodID != "" {
		return event.ProviderPaymentMethodID
	}

	return event.ProviderPaymentID
}

func (s *Service) subscriptionPeriodForWebhook(ctx context.Context, q *dbgen.Queries, userID uuid.UUID, paymentType string, paidAt time.Time, period PlanPeriod) (time.Time, time.Time, error) {
	if paymentType != paymentTypeSubscriptionRenewal {
		start, end := billingPeriodForPlan(paidAt, period)
		return start, end, nil
	}

	current, err := q.GetCurrentSubscriptionByUserID(ctx, userID)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("get current subscription for renewal: %w", err)
	}

	start := current.CurrentPeriodEnd.Time.UTC()
	return start, billingPeriodEnd(start, period), nil
}

// activateAddon зачисляет дополнительные карточки после покупки addon-пакета.
func (s *Service) activateAddon(ctx context.Context, q *dbgen.Queries, event WebhookEvent, paidAt pgtype.Timestamptz) error {
	userID, err := parseUserID(event.Metadata.UserID)
	if err != nil {
		return fmt.Errorf("parse user_id from webhook addon metadata: %w", err)
	}

	addon, err := q.GetAddonProductByCode(ctx, event.Metadata.AddonCode)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("addon %q not found for webhook activation", event.Metadata.AddonCode)
		}
		return fmt.Errorf("get addon for webhook activation: %w", err)
	}

	affected, err := q.AddAddonCardsToQuota(ctx, dbgen.AddAddonCardsToQuotaParams{
		ExtraCards: addon.CardsCount,
		UserID:     userID,
		NowAt:      paidAt,
	})
	if err != nil {
		return fmt.Errorf("add addon cards to quota: %w", err)
	}
	if affected == 0 {
		// Активной квоты нет — addon оплачен, но подписки нет.
		// Возвращаем ошибку, чтобы транзакция откатилась и ЮКасса повторила уведомление позже,
		// когда у пользователя появится активная подписка.
		return fmt.Errorf("no active usage quota for user %s to credit addon %q", userID, addon.Code)
	}

	return nil
}

// handlePaymentCanceled помечает платёж как отменённый.
// affected == 0 означает, что платёж уже был отменён ранее — повторный webhook безопасен.
func (s *Service) handlePaymentCanceled(ctx context.Context, event WebhookEvent) error {
	_, err := s.queries.MarkPaymentCanceled(ctx, toPgText(event.ProviderPaymentID))
	if err != nil {
		return fmt.Errorf("mark payment canceled: %w", err)
	}

	return nil
}

// billingPeriodForPlan вычисляет период подписки от момента активации в зависимости от плана.
func billingPeriodForPlan(from time.Time, period PlanPeriod) (time.Time, time.Time) {
	start := from.UTC().Truncate(24 * time.Hour)
	return start, billingPeriodEnd(start, period)
}

func billingPeriodEnd(start time.Time, period PlanPeriod) time.Time {
	switch period {
	case PlanPeriodYearly:
		return start.AddDate(1, 0, 0)
	default:
		return start.AddDate(0, 1, 0)
	}
}
