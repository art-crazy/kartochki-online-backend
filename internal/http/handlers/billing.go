package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/rs/zerolog"

	openapi "kartochki-online-backend/api/gen"
	"kartochki-online-backend/internal/billing"
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
	user, ok := currentUserFromCtx(w, r)
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
	user, ok := currentUserFromCtx(w, r)
	if !ok {
		return
	}

	var req openapi.CreateCheckoutRequest
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
		PlanID: req.PlanId,
		Period: billing.PlanPeriod(req.Period),
	})
	if err != nil {
		h.writeBillingError(w, r, err, "failed to create billing checkout")
		return
	}

	response.WriteJSON(w, r, http.StatusAccepted, openapi.CreateCheckoutResponse{
		CheckoutUrl: result.CheckoutURL,
	})
}

// PurchaseAddon создаёт checkout для разового пакета карточек.
func (h BillingHandler) PurchaseAddon(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUserFromCtx(w, r)
	if !ok {
		return
	}

	var req openapi.PurchaseAddonRequest
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
		AddonID: req.AddonId,
	})
	if err != nil {
		h.writeBillingError(w, r, err, "failed to create addon checkout")
		return
	}

	response.WriteJSON(w, r, http.StatusAccepted, openapi.PurchaseAddonResponse{
		CheckoutUrl: result.CheckoutURL,
	})
}

// CancelSubscription отключает автопродление платной подписки.
func (h BillingHandler) CancelSubscription(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUserFromCtx(w, r)
	if !ok {
		return
	}

	if err := h.service.CancelSubscription(r.Context(), user.ID); err != nil {
		h.writeBillingError(w, r, err, "failed to cancel subscription")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, openapi.CancelSubscriptionResponse{
		Status: openapi.ScheduledCancel,
	})
}

func validateCreateCheckoutRequest(req openapi.CreateCheckoutRequest) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	if strings.TrimSpace(req.PlanId) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("plan_id"), Message: "field is required"})
	}
	if strings.TrimSpace(string(req.Period)) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("period"), Message: "field is required"})
	}

	return details
}

func validatePurchaseAddonRequest(req openapi.PurchaseAddonRequest) []openapi.ErrorDetail {
	if strings.TrimSpace(req.AddonId) == "" {
		return []openapi.ErrorDetail{{Field: strPtr("addon_id"), Message: "field is required"}}
	}

	return nil
}

func toBillingResponse(payload billing.Billing) openapi.BillingResponse {
	return openapi.BillingResponse{
		CurrentSubscription: openapi.BillingSubscription{
			PlanId:           payload.CurrentSubscription.PlanID,
			PlanName:         payload.CurrentSubscription.PlanName,
			RenewsAt:         payload.CurrentSubscription.RenewsAt,
			CancelsAt:        payload.CurrentSubscription.CancelsAt,
			HasPaymentMethod: payload.CurrentSubscription.HasPaymentMethod,
			Usage: openapi.BillingUsage{
				Value: payload.CurrentSubscription.Usage.Value,
				Max:   payload.CurrentSubscription.Usage.Max,
			},
		},
		Plans:  toBillingPlans(payload.Plans),
		Addons: toBillingAddons(payload.Addons),
	}
}

func toBillingPlans(items []billing.Plan) []openapi.BillingPlan {
	result := make([]openapi.BillingPlan, len(items))
	for i, item := range items {
		plan := openapi.BillingPlan{
			Id:            item.ID,
			Name:          item.Name,
			MonthlyPrice:  item.MonthlyPrice,
			CardsPerMonth: item.CardsPerMonth,
			Features:      toBillingPlanFeatures(item.Features),
		}
		if item.YearlyMonthlyPrice > 0 {
			plan.YearlyMonthlyPrice = &item.YearlyMonthlyPrice
		}
		if item.Current {
			plan.Current = &item.Current
		}
		if item.Popular {
			plan.Popular = &item.Popular
		}
		result[i] = plan
	}

	return result
}

func toBillingPlanFeatures(items []billing.PlanFeature) []openapi.BillingPlanFeature {
	result := make([]openapi.BillingPlanFeature, len(items))
	for i, item := range items {
		result[i] = openapi.BillingPlanFeature{
			Label:   item.Label,
			Enabled: item.Enabled,
		}
	}

	return result
}

func toBillingAddons(items []billing.Addon) []openapi.BillingAddon {
	result := make([]openapi.BillingAddon, len(items))
	for i, item := range items {
		result[i] = openapi.BillingAddon{
			Id:          item.ID,
			Title:       item.Title,
			Description: item.Description,
			Price:       item.Price,
		}
	}

	return result
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
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", openapi.ErrorDetail{
			Field:   strPtr("period"),
			Message: "unsupported billing period",
		})
	case errors.Is(err, billing.ErrPlanAlreadyActive):
		response.WriteError(w, r, http.StatusConflict, "plan_already_active", "billing plan is already active")
	case errors.Is(err, billing.ErrSubscriptionNotCancelable):
		response.WriteError(w, r, http.StatusConflict, "subscription_not_cancelable", "subscription cannot be canceled")
	case errors.Is(err, billing.ErrCheckoutProviderNotConfigured):
		response.WriteError(w, r, http.StatusNotImplemented, "checkout_not_configured", "checkout provider is not configured yet")
	case errors.Is(err, billing.ErrCheckoutProviderFailed):
		response.WriteError(w, r, http.StatusBadGateway, "checkout_provider_error", err.Error())
	case errors.Is(err, billing.ErrCheckoutPersistenceFailed):
		response.WriteError(w, r, http.StatusInternalServerError, "checkout_persistence_error", err.Error())
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
