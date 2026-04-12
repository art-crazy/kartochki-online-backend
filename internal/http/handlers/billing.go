package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/rs/zerolog"

	"kartochki-online-backend/internal/auth"
	"kartochki-online-backend/internal/billing"
	"kartochki-online-backend/internal/http/authctx"
	"kartochki-online-backend/internal/http/contracts"
	"kartochki-online-backend/internal/http/requestctx"
	"kartochki-online-backend/internal/http/response"
)

// billingService описывает billing-сценарии, доступные HTTP-слою.
type billingService interface {
	Get(ctx context.Context, userID string) (billing.Billing, error)
	CreateCheckout(ctx context.Context, input billing.CheckoutInput) (billing.CheckoutResult, error)
	PurchaseAddon(ctx context.Context, input billing.PurchaseAddonInput) (billing.PurchaseAddonResult, error)
	CancelSubscription(ctx context.Context, userID string) error
}

// BillingHandler обслуживает `/api/v1/billing` и связанные checkout-сценарии.
type BillingHandler struct {
	service billingService
	logger  zerolog.Logger
}

// NewBillingHandler создаёт billing handler.
func NewBillingHandler(service billingService, logger zerolog.Logger) BillingHandler {
	return BillingHandler{
		service: service,
		logger:  logger,
	}
}

// Get возвращает агрегированный billing-ответ для страницы `/app/billing`.
func (h BillingHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentUser(w, r)
	if !ok {
		return
	}

	payload, err := h.service.Get(r.Context(), user.ID)
	if err != nil {
		h.writeBillingError(w, r, err, "failed to load billing")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, toBillingResponse(payload))
}

// CreateCheckout создаёт checkout для смены тарифа.
func (h BillingHandler) CreateCheckout(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentUser(w, r)
	if !ok {
		return
	}

	var req contracts.CreateCheckoutRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if details := validateCreateCheckoutRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", details...)
		return
	}

	result, err := h.service.CreateCheckout(r.Context(), billing.CheckoutInput{
		UserID: user.ID,
		PlanID: req.PlanID,
		Period: billing.PlanPeriod(req.Period),
	})
	if err != nil {
		h.writeBillingError(w, r, err, "failed to create billing checkout")
		return
	}

	response.WriteJSON(w, r, http.StatusAccepted, contracts.CreateCheckoutResponse{
		CheckoutURL: result.CheckoutURL,
	})
}

// PurchaseAddon создаёт checkout для разового пакета карточек.
func (h BillingHandler) PurchaseAddon(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentUser(w, r)
	if !ok {
		return
	}

	var req contracts.PurchaseAddonRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if details := validatePurchaseAddonRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", details...)
		return
	}

	result, err := h.service.PurchaseAddon(r.Context(), billing.PurchaseAddonInput{
		UserID:  user.ID,
		AddonID: req.AddonID,
	})
	if err != nil {
		h.writeBillingError(w, r, err, "failed to create addon checkout")
		return
	}

	response.WriteJSON(w, r, http.StatusAccepted, contracts.PurchaseAddonResponse{
		CheckoutURL: result.CheckoutURL,
	})
}

// CancelSubscription отключает автопродление платной подписки.
func (h BillingHandler) CancelSubscription(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentUser(w, r)
	if !ok {
		return
	}

	if err := h.service.CancelSubscription(r.Context(), user.ID); err != nil {
		h.writeBillingError(w, r, err, "failed to cancel subscription")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, contracts.CancelSubscriptionResponse{
		Status: "scheduled_cancel",
	})
}

func validateCreateCheckoutRequest(req contracts.CreateCheckoutRequest) []contracts.ErrorDetail {
	var details []contracts.ErrorDetail

	if strings.TrimSpace(req.PlanID) == "" {
		details = append(details, contracts.ErrorDetail{Field: "plan_id", Message: "field is required"})
	}
	if strings.TrimSpace(string(req.Period)) == "" {
		details = append(details, contracts.ErrorDetail{Field: "period", Message: "field is required"})
	}

	return details
}

func validatePurchaseAddonRequest(req contracts.PurchaseAddonRequest) []contracts.ErrorDetail {
	if strings.TrimSpace(req.AddonID) == "" {
		return []contracts.ErrorDetail{{Field: "addon_id", Message: "field is required"}}
	}

	return nil
}

func toBillingResponse(payload billing.Billing) contracts.BillingResponse {
	return contracts.BillingResponse{
		CurrentSubscription: contracts.BillingSubscription{
			PlanID:           payload.CurrentSubscription.PlanID,
			PlanName:         payload.CurrentSubscription.PlanName,
			RenewsAt:         payload.CurrentSubscription.RenewsAt,
			CancelsAt:        payload.CurrentSubscription.CancelsAt,
			HasPaymentMethod: payload.CurrentSubscription.HasPaymentMethod,
			Usage: contracts.BillingUsage{
				Value: payload.CurrentSubscription.Usage.Value,
				Max:   payload.CurrentSubscription.Usage.Max,
			},
		},
		Plans:  toBillingPlans(payload.Plans),
		Addons: toBillingAddons(payload.Addons),
	}
}

func toBillingPlans(items []billing.Plan) []contracts.BillingPlan {
	result := make([]contracts.BillingPlan, len(items))
	for i, item := range items {
		result[i] = contracts.BillingPlan{
			ID:                 item.ID,
			Name:               item.Name,
			MonthlyPrice:       item.MonthlyPrice,
			YearlyMonthlyPrice: item.YearlyMonthlyPrice,
			CardsPerMonth:      item.CardsPerMonth,
			Features:           toBillingPlanFeatures(item.Features),
			Current:            item.Current,
			Popular:            item.Popular,
		}
	}

	return result
}

func toBillingPlanFeatures(items []billing.PlanFeature) []contracts.BillingPlanFeature {
	result := make([]contracts.BillingPlanFeature, len(items))
	for i, item := range items {
		result[i] = contracts.BillingPlanFeature{
			Label:   item.Label,
			Enabled: item.Enabled,
		}
	}

	return result
}

func toBillingAddons(items []billing.Addon) []contracts.BillingAddon {
	result := make([]contracts.BillingAddon, len(items))
	for i, item := range items {
		result[i] = contracts.BillingAddon{
			ID:          item.ID,
			Title:       item.Title,
			Description: item.Description,
			Price:       item.Price,
		}
	}

	return result
}

func (h BillingHandler) currentUser(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	user, ok := authctx.User(r.Context())
	if !ok {
		response.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "authorization token is invalid")
		return auth.User{}, false
	}

	return user, true
}

func (h BillingHandler) writeBillingError(w http.ResponseWriter, r *http.Request, err error, fallbackMessage string) {
	switch {
	case errors.Is(err, billing.ErrUserNotFound):
		response.WriteError(w, r, http.StatusNotFound, "user_not_found", "user not found")
	case errors.Is(err, billing.ErrPlanNotFound):
		response.WriteError(w, r, http.StatusNotFound, "plan_not_found", "billing plan not found")
	case errors.Is(err, billing.ErrAddonNotFound):
		response.WriteError(w, r, http.StatusNotFound, "addon_not_found", "billing addon not found")
	case errors.Is(err, billing.ErrInvalidPlanPeriod):
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", contracts.ErrorDetail{
			Field:   "period",
			Message: "unsupported billing period",
		})
	case errors.Is(err, billing.ErrPlanAlreadyActive):
		response.WriteError(w, r, http.StatusConflict, "plan_already_active", "billing plan is already active")
	case errors.Is(err, billing.ErrSubscriptionNotCancelable):
		response.WriteError(w, r, http.StatusConflict, "subscription_not_cancelable", "subscription cannot be canceled")
	case errors.Is(err, billing.ErrCheckoutProviderNotConfigured):
		response.WriteError(w, r, http.StatusNotImplemented, "checkout_not_configured", "checkout provider is not configured yet")
	case errors.Is(err, billing.ErrFreePlanNotFound):
		// Бесплатный тариф отсутствует в БД — миграция не применена.
		// Логируем как критическую ошибку конфигурации, клиенту отдаём 503.
		logger := requestctx.Logger(r.Context(), h.logger)
		logger.Error().Err(err).Msg("план free отсутствует в таблице plans — миграция не применена")
		response.WriteError(w, r, http.StatusServiceUnavailable, "billing_misconfigured", "billing is not properly configured")
	default:
		logger := requestctx.Logger(r.Context(), h.logger)
		logger.Error().Err(err).Msg("не удалось выполнить billing-сценарий")
		response.WriteError(w, r, http.StatusInternalServerError, "internal_error", fallbackMessage)
	}
}
