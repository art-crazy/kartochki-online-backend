package handlers

import (
	"errors"
	"net/http"

	openapi "kartochki-online-backend/api/gen"
	"kartochki-online-backend/internal/billing"
	"kartochki-online-backend/internal/http/requestctx"
	"kartochki-online-backend/internal/http/response"
)

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
	case errors.Is(err, billing.ErrPaymentMethodNotFound):
		response.WriteError(w, r, http.StatusNotFound, "payment_method_not_found", "no saved payment method found")
	case errors.Is(err, billing.ErrCheckoutProviderNotConfigured):
		response.WriteError(w, r, http.StatusNotImplemented, "checkout_not_configured", "checkout provider is not configured yet")
	case errors.Is(err, billing.ErrCheckoutProviderFailed):
		response.WriteError(w, r, http.StatusBadGateway, "checkout_provider_error", err.Error())
	case errors.Is(err, billing.ErrCheckoutPersistenceFailed):
		response.WriteError(w, r, http.StatusInternalServerError, "checkout_persistence_error", err.Error())
	case errors.Is(err, billing.ErrCheckoutEmailRequired):
		response.WriteError(w, r, http.StatusConflict, "email_required", "Добавьте email в настройках профиля, чтобы перейти к оплате.", openapi.ErrorDetail{
			Field:   strPtr("email"),
			Message: "Email нужен для формирования чека ЮКасса.",
		})
	case errors.Is(err, billing.ErrCheckoutPreparationFailed):
		response.WriteError(w, r, http.StatusInternalServerError, "checkout_preparation_error", err.Error())
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
