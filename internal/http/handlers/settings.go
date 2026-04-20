package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/rs/zerolog"

	openapi "kartochki-online-backend/api/gen"
	"kartochki-online-backend/internal/auth"
	"kartochki-online-backend/internal/http/authctx"
	"kartochki-online-backend/internal/http/requestctx"
	"kartochki-online-backend/internal/http/response"
	"kartochki-online-backend/internal/settings"
)

// settingsService описывает бизнес-операции страницы настроек.
type settingsService interface {
	PasswordMinLength() int
	Get(ctx context.Context, userID string, currentAccessToken string) (settings.Settings, error)
	UpdateProfile(ctx context.Context, userID string, input settings.UpdateProfileInput) (settings.Profile, error)
	UploadAvatar(ctx context.Context, userID string, image settings.UploadedAvatar) (settings.UploadedAvatarResult, error)
	UpdateDefaults(ctx context.Context, userID string, input settings.UpdateDefaultsInput) (settings.Defaults, error)
	UpdateNotifications(ctx context.Context, userID string, items []settings.NotificationItem) ([]settings.NotificationItem, error)
	ChangePassword(ctx context.Context, userID string, currentAccessToken string, currentPassword string, newPassword string) error
	DeleteSession(ctx context.Context, userID string, sessionID string, currentAccessToken string) error
	RotateAPIKey(ctx context.Context, userID string) (settings.RotatedAPIKey, error)
	ExportData(ctx context.Context, userID string) error
	DeleteAccount(ctx context.Context, userID string, confirmWord string) error
}

// SettingsHandler обслуживает `/api/v1/settings` и связанные security-операции.
type SettingsHandler struct {
	settingsService settingsService
	logger          zerolog.Logger
}

// NewSettingsHandler создаёт обработчик страницы настроек.
func NewSettingsHandler(settingsService settingsService, logger zerolog.Logger) SettingsHandler {
	return SettingsHandler{settingsService: settingsService, logger: logger}
}

// Get возвращает агрегированный ответ для страницы `/app/settings`.
func (h SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, token, ok := h.currentAuth(w, r)
	if !ok {
		return
	}

	payload, err := h.settingsService.Get(r.Context(), user.ID, token)
	if err != nil {
		h.writeSettingsError(w, r, err, "failed to load settings")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, toSettingsResponse(payload))
}

// PatchProfile обновляет профиль пользователя.
func (h SettingsHandler) PatchProfile(w http.ResponseWriter, r *http.Request) {
	user, _, ok := h.currentAuth(w, r)
	if !ok {
		return
	}

	var req openapi.UpdateProfileRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if details := validateUpdateProfileRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", details...)
		return
	}

	profile, err := h.settingsService.UpdateProfile(r.Context(), user.ID, settings.UpdateProfileInput{
		Name:    req.Name,
		Email:   string(req.Email),
		Phone:   stringOrEmpty(req.Phone),
		Company: stringOrEmpty(req.Company),
	})
	if err != nil {
		switch {
		case errors.Is(err, settings.ErrEmailTaken):
			response.WriteError(w, r, http.StatusConflict, "email_taken", "email is already registered")
		case errors.Is(err, settings.ErrNameRequired):
			response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", openapi.ErrorDetail{
				Field:   strPtr("name"),
				Message: "field is required",
			})
		default:
			h.writeSettingsError(w, r, err, "failed to update profile")
		}
		return
	}

	response.WriteJSON(w, r, http.StatusOK, toSettingsProfileContract(profile))
}

// PatchDefaults обновляет дефолтные параметры генерации.
func (h SettingsHandler) PatchDefaults(w http.ResponseWriter, r *http.Request) {
	user, _, ok := h.currentAuth(w, r)
	if !ok {
		return
	}

	var req openapi.UpdateDefaultsRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if details := validateUpdateDefaultsRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", details...)
		return
	}

	defaults, err := h.settingsService.UpdateDefaults(r.Context(), user.ID, settings.UpdateDefaultsInput{
		MarketplaceID:      req.MarketplaceId,
		CardsPerGeneration: req.CardsPerGeneration,
		Format:             string(req.Format),
	})
	if err != nil {
		switch {
		case errors.Is(err, settings.ErrCardsPerGenerationOutOfRange):
			response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", openapi.ErrorDetail{
				Field:   strPtr("cards_per_generation"),
				Message: "must be between 1 and 50",
			})
		case errors.Is(err, settings.ErrInvalidImageFormat):
			response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", openapi.ErrorDetail{
				Field:   strPtr("format"),
				Message: "must be one of: png, jpg, webp",
			})
		default:
			h.writeSettingsError(w, r, err, "failed to update defaults")
		}
		return
	}

	response.WriteJSON(w, r, http.StatusOK, openapi.SettingsDefaults{
		MarketplaceId:      defaults.MarketplaceID,
		CardsPerGeneration: defaults.CardsPerGeneration,
		Format:             openapi.SettingsDefaultsFormat(defaults.Format),
	})
}

// ChangePassword меняет локальный пароль и отзывает остальные сессии.
func (h SettingsHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user, token, ok := h.currentAuth(w, r)
	if !ok {
		return
	}

	var req openapi.ChangePasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if details := validateChangePasswordRequest(req, h.settingsService.PasswordMinLength()); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", details...)
		return
	}

	if err := h.settingsService.ChangePassword(r.Context(), user.ID, token, req.CurrentPassword, req.NewPassword); err != nil {
		switch {
		case errors.Is(err, settings.ErrCurrentPasswordInvalid):
			response.WriteError(w, r, http.StatusBadRequest, "invalid_current_password", "current password is invalid")
		case errors.Is(err, auth.ErrPasswordTooShort):
			response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", openapi.ErrorDetail{
				Field:   strPtr("new_password"),
				Message: fmt.Sprintf("must be at least %d characters", h.settingsService.PasswordMinLength()),
			})
		default:
			h.writeSettingsError(w, r, err, "failed to change password")
		}
		return
	}

	response.WriteJSON(w, r, http.StatusOK, openapi.StatusResponse{Status: openapi.StatusResponseStatusPasswordChanged})
}

// PatchNotifications сохраняет переключатели уведомлений.
func (h SettingsHandler) PatchNotifications(w http.ResponseWriter, r *http.Request) {
	user, _, ok := h.currentAuth(w, r)
	if !ok {
		return
	}

	var req openapi.UpdateNotificationsRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if details := validateUpdateNotificationsRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", details...)
		return
	}

	items := make([]settings.NotificationItem, len(req.Items))
	for i, item := range req.Items {
		items[i] = settings.NotificationItem{
			Key:     item.Key,
			Enabled: item.Enabled,
		}
	}

	updated, err := h.settingsService.UpdateNotifications(r.Context(), user.ID, items)
	if err != nil {
		if errors.Is(err, settings.ErrUnknownNotificationKey) {
			response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", openapi.ErrorDetail{
				Field:   strPtr("items"),
				Message: "contains unknown notification key",
			})
			return
		}

		h.writeSettingsError(w, r, err, "failed to update notifications")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, openapi.SettingsNotifications{Items: toNotificationContracts(updated)})
}

// DeleteSession отзывает одну не-текущую сессию пользователя.
func (h SettingsHandler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	user, token, ok := h.currentAuth(w, r)
	if !ok {
		return
	}

	sessionID := strings.TrimSpace(chi.URLParam(r, "id"))
	if sessionID == "" {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "session id is required")
		return
	}

	if err := h.settingsService.DeleteSession(r.Context(), user.ID, sessionID, token); err != nil {
		switch {
		case errors.Is(err, settings.ErrCannotRevokeCurrentSession):
			response.WriteError(w, r, http.StatusBadRequest, "cannot_revoke_current_session", "current session cannot be revoked")
		case errors.Is(err, settings.ErrSessionNotFound):
			response.WriteError(w, r, http.StatusNotFound, "session_not_found", "session not found")
		default:
			h.writeSettingsError(w, r, err, "failed to revoke session")
		}
		return
	}

	response.WriteJSON(w, r, http.StatusOK, openapi.StatusResponse{Status: openapi.StatusResponseStatusDeleted})
}

// RotateAPIKey перевыпускает API-ключ и отдаёт новый секрет один раз.
func (h SettingsHandler) RotateAPIKey(w http.ResponseWriter, r *http.Request) {
	user, _, ok := h.currentAuth(w, r)
	if !ok {
		return
	}

	key, err := h.settingsService.RotateAPIKey(r.Context(), user.ID)
	if err != nil {
		h.writeSettingsError(w, r, err, "failed to rotate api key")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, openapi.RotateAPIKeyResponse{
		MaskedValue: key.MaskedValue,
		PlainValue:  key.PlainValue,
	})
}

// ExportData ставит задачу экспорта пользовательских данных в очередь.
func (h SettingsHandler) ExportData(w http.ResponseWriter, r *http.Request) {
	user, _, ok := h.currentAuth(w, r)
	if !ok {
		return
	}

	if err := h.settingsService.ExportData(r.Context(), user.ID); err != nil {
		h.writeSettingsError(w, r, err, "failed to enqueue account export")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, openapi.ExportDataResponse{Status: openapi.ExportDataResponseStatusAccepted})
}

// DeleteAccount удаляет аккаунт после явного подтверждения.
func (h SettingsHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	user, _, ok := h.currentAuth(w, r)
	if !ok {
		return
	}

	var req openapi.DeleteAccountRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if strings.TrimSpace(req.ConfirmWord) == "" {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", openapi.ErrorDetail{
			Field:   strPtr("confirm_word"),
			Message: "field is required",
		})
		return
	}

	if err := h.settingsService.DeleteAccount(r.Context(), user.ID, req.ConfirmWord); err != nil {
		if errors.Is(err, settings.ErrInvalidConfirmWord) {
			response.WriteError(w, r, http.StatusBadRequest, "invalid_confirm_word", "confirm word is invalid")
			return
		}

		h.writeSettingsError(w, r, err, "failed to delete account")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, openapi.StatusResponse{Status: openapi.StatusResponseStatusDeleted})
}

func (h SettingsHandler) currentAuth(w http.ResponseWriter, r *http.Request) (auth.User, string, bool) {
	user, ok := authctx.User(r.Context())
	if !ok {
		response.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "authorization token is invalid")
		return auth.User{}, "", false
	}

	token, ok := authctx.AccessToken(r.Context())
	if !ok {
		response.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "authorization token is required")
		return auth.User{}, "", false
	}

	return user, token, true
}

func (h SettingsHandler) writeSettingsError(w http.ResponseWriter, r *http.Request, err error, publicMessage string) {
	if errors.Is(err, settings.ErrUserNotFound) {
		response.WriteError(w, r, http.StatusNotFound, "user_not_found", "user not found")
		return
	}

	logger := requestctx.Logger(r.Context(), h.logger)
	logger.Error().Err(err).Msg("не удалось выполнить settings-сценарий")
	response.WriteError(w, r, http.StatusInternalServerError, "internal_error", publicMessage)
}

func toSettingsResponse(payload settings.Settings) openapi.SettingsResponse {
	apiKey := openapi.SettingsAPIKey{
		CanRotate: payload.APIKey.CanRotate,
	}
	if payload.APIKey.MaskedValue != "" {
		apiKey.MaskedValue = &payload.APIKey.MaskedValue
	}

	return openapi.SettingsResponse{
		Profile: toSettingsProfileContract(payload.Profile),
		Defaults: openapi.SettingsDefaults{
			MarketplaceId:      payload.Defaults.MarketplaceID,
			CardsPerGeneration: payload.Defaults.CardsPerGeneration,
			Format:             openapi.SettingsDefaultsFormat(payload.Defaults.Format),
		},
		Notifications: openapi.SettingsNotifications{Items: toNotificationContracts(payload.Notifications)},
		Sessions:      toSettingsSessionContracts(payload.Sessions),
		Integrations:  toSettingsIntegrationContracts(payload.Integrations),
		ApiKey:        apiKey,
	}
}

// toSettingsProfileContract собирает профиль настроек в OpenAPI-ответ.
// Email остаётся опциональным, чтобы битые исторические данные не ломали JSON-ответ.
func toSettingsProfileContract(profile settings.Profile) openapi.SettingsProfile {
	result := openapi.SettingsProfile{
		Name: profile.Name,
	}
	if email := contractEmailPtr(profile.Email); email != nil {
		result.Email = email
	}
	if profile.Phone != "" {
		result.Phone = &profile.Phone
	}
	if profile.Company != "" {
		result.Company = &profile.Company
	}

	return result
}

func toNotificationContracts(items []settings.NotificationItem) []openapi.UpdateNotificationItem {
	result := make([]openapi.UpdateNotificationItem, len(items))
	for i, item := range items {
		result[i] = openapi.UpdateNotificationItem{
			Key:     item.Key,
			Enabled: item.Enabled,
		}
	}

	return result
}

func toSettingsSessionContracts(items []settings.Session) []openapi.SettingsSession {
	result := make([]openapi.SettingsSession, len(items))
	for i, item := range items {
		s := openapi.SettingsSession{
			Id:        mustParseUUID(item.ID),
			Device:    item.Device,
			Platform:  item.Platform,
			IsCurrent: item.IsCurrent,
			CanRevoke: item.CanRevoke,
		}
		if item.Location != "" {
			s.Location = &item.Location
		}
		result[i] = s
	}

	return result
}

func toSettingsIntegrationContracts(items []settings.Integration) []openapi.SettingsIntegration {
	result := make([]openapi.SettingsIntegration, len(items))
	for i, item := range items {
		s := openapi.SettingsIntegration{
			Provider:  item.Provider,
			Connected: item.Connected,
		}
		// ID и AccountEmail в интеграции опциональны — не подключённые провайдеры могут не иметь ID.
		if item.ID != "" {
			id := mustParseUUID(item.ID)
			s.Id = &id
		}
		// Email в snapshot OAuth-аккаунта может отсутствовать или быть невалидным.
		// В таком случае не роняем весь `/settings`, а просто не отдаём это поле.
		if email := contractEmailPtr(item.AccountEmail); email != nil {
			s.AccountEmail = email
		}
		result[i] = s
	}

	return result
}

// contractEmailPtr возвращает email только если он подходит под OpenAPI-тип.
// Это защищает JSON-ответ от падения на пустых или повреждённых значениях из БД.
func contractEmailPtr(value string) *openapi_types.Email {
	value = strings.TrimSpace(value)
	if value == "" || !isValidEmail(value) {
		return nil
	}

	email := openapi_types.Email(value)
	return &email
}

func validateUpdateProfileRequest(req openapi.UpdateProfileRequest) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	if strings.TrimSpace(req.Name) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("name"), Message: "field is required"})
	}
	if email := strings.TrimSpace(string(req.Email)); email != "" && !isValidEmail(email) {
		details = append(details, openapi.ErrorDetail{Field: strPtr("email"), Message: "must be a valid email"})
	}

	return details
}

func validateUpdateDefaultsRequest(req openapi.UpdateDefaultsRequest) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	if req.CardsPerGeneration < 1 || req.CardsPerGeneration > 50 {
		details = append(details, openapi.ErrorDetail{Field: strPtr("cards_per_generation"), Message: "must be between 1 and 50"})
	}

	format := strings.TrimSpace(strings.ToLower(string(req.Format)))
	if format == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("format"), Message: "field is required"})
	} else if format != "png" && format != "jpg" && format != "webp" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("format"), Message: "must be one of: png, jpg, webp"})
	}

	return details
}

func validateChangePasswordRequest(req openapi.ChangePasswordRequest, passwordMinLength int) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	if strings.TrimSpace(req.CurrentPassword) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("current_password"), Message: "field is required"})
	}
	if strings.TrimSpace(req.NewPassword) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("new_password"), Message: "field is required"})
	} else if len(req.NewPassword) < passwordMinLength {
		details = append(details, openapi.ErrorDetail{
			Field:   strPtr("new_password"),
			Message: fmt.Sprintf("must be at least %d characters", passwordMinLength),
		})
	}

	return details
}

func validateUpdateNotificationsRequest(req openapi.UpdateNotificationsRequest) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	if len(req.Items) == 0 {
		details = append(details, openapi.ErrorDetail{Field: strPtr("items"), Message: "must contain at least one item"})
		return details
	}

	seen := make(map[string]struct{}, len(req.Items))
	for _, item := range req.Items {
		key := strings.TrimSpace(item.Key)
		if key == "" {
			details = append(details, openapi.ErrorDetail{Field: strPtr("items.key"), Message: "field is required"})
			continue
		}
		if _, ok := seen[key]; ok {
			details = append(details, openapi.ErrorDetail{Field: strPtr("items.key"), Message: "must be unique"})
			continue
		}
		seen[key] = struct{}{}
	}

	return details
}
