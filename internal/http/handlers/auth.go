package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	openapi "kartochki-online-backend/api/gen"
	"kartochki-online-backend/internal/auth"
	"kartochki-online-backend/internal/http/authctx"
	"kartochki-online-backend/internal/http/response"
)

// AuthHandler обслуживает публичные auth-сценарии и маршруты текущего пользователя.
type AuthHandler struct {
	authService *auth.Service
}

// NewAuthHandler создаёт обработчик auth endpoint.
func NewAuthHandler(authService *auth.Service) AuthHandler {
	return AuthHandler{authService: authService}
}

// Register создаёт пользователя и сразу логинит его в первую сессию.
func (h AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req openapi.RegisterRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if details := validateRegisterRequest(req, h.authService.PasswordMinLength()); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", details...)
		return
	}

	result, err := h.authService.Register(r.Context(), auth.RegisterInput{
		Name:     stringOrEmpty(req.Name),
		Email:    string(req.Email),
		Password: req.Password,
	}, sessionMetadataFromRequest(r))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrEmailAlreadyExists):
			response.WriteError(w, r, http.StatusConflict, "email_taken", "email is already registered")
		case errors.Is(err, auth.ErrPasswordTooShort):
			response.WriteError(
				w,
				r,
				http.StatusBadRequest,
				"validation_error",
				"request validation failed",
				openapi.ErrorDetail{
					Field:   strPtr("password"),
					Message: fmt.Sprintf("must be at least %d characters", h.authService.PasswordMinLength()),
				},
			)
		default:
			response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to register user")
		}
		return
	}

	response.WriteJSON(w, r, http.StatusCreated, toAuthResponse(result))
}

// Login создаёт новую сессию по корректной паре email и пароль.
func (h AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req openapi.LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if details := validateLoginRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", details...)
		return
	}

	result, err := h.authService.Login(r.Context(), auth.LoginInput{
		Email:    string(req.Email),
		Password: req.Password,
	}, sessionMetadataFromRequest(r))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			response.WriteError(w, r, http.StatusUnauthorized, "invalid_credentials", "email or password is invalid")
		default:
			response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to login")
		}
		return
	}

	response.WriteJSON(w, r, http.StatusOK, toAuthResponse(result))
}

// TelegramLogin завершает вход через Telegram Login Widget и создаёт локальную сессию.
func (h AuthHandler) TelegramLogin(w http.ResponseWriter, r *http.Request) {
	var req openapi.TelegramLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if details := validateTelegramLoginRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", details...)
		return
	}

	result, err := h.authService.LoginWithTelegram(r.Context(), auth.TelegramLoginData{
		ID:        req.Id,
		FirstName: stringOrEmpty(req.FirstName),
		LastName:  stringOrEmpty(req.LastName),
		Username:  stringOrEmpty(req.Username),
		PhotoURL:  stringOrEmpty(req.PhotoUrl),
		AuthDate:  req.AuthDate,
		Hash:      req.Hash,
	}, sessionMetadataFromRequest(r))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrTelegramAuthNotConfigured):
			response.WriteError(w, r, http.StatusNotImplemented, "telegram_not_configured", "telegram auth is not configured yet")
		case errors.Is(err, auth.ErrTelegramAuthInvalid):
			response.WriteError(w, r, http.StatusBadRequest, "invalid_telegram_auth", "telegram auth payload is invalid")
		case errors.Is(err, auth.ErrTelegramAuthExpired):
			response.WriteError(w, r, http.StatusBadRequest, "telegram_auth_expired", "telegram auth payload is expired")
		default:
			response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to login with telegram")
		}
		return
	}

	response.WriteJSON(w, r, http.StatusOK, toAuthResponse(result))
}

// Logout отзывает текущую сессию по Bearer-токену.
func (h AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	token, ok := authctx.AccessToken(r.Context())
	if !ok {
		response.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "authorization token is required")
		return
	}

	if err := h.authService.Logout(r.Context(), token); err != nil {
		switch {
		case errors.Is(err, auth.ErrUnauthorized):
			response.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "authorization token is invalid")
		default:
			response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to logout")
		}
		return
	}

	response.WriteJSON(w, r, http.StatusOK, openapi.StatusResponse{Status: openapi.StatusResponseStatusLoggedOut})
}

// Me возвращает текущего пользователя, который уже был загружен middleware.
func (h AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUserFromCtx(w, r)
	if !ok {
		return
	}

	response.WriteJSON(w, r, http.StatusOK, openapi.CurrentUserResponse{
		User: authUserToAPI(user),
	})
}

// ForgotPassword принимает email и инициирует отправку письма со ссылкой для сброса пароля.
//
// Ответ всегда 200 независимо от того, найден пользователь или нет — чтобы не раскрывать
// факт существования аккаунта по email.
func (h AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req openapi.ForgotPasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if details := validateForgotPasswordRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", details...)
		return
	}

	if err := h.authService.ForgotPassword(r.Context(), string(req.Email)); err != nil {
		response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to process password reset request")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, openapi.StatusResponse{Status: openapi.StatusResponseStatusAccepted})
}

// ResetPassword принимает токен из письма и новый пароль, затем обновляет пароль пользователя.
func (h AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req openapi.ResetPasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if details := validateResetPasswordRequest(req, h.authService.PasswordMinLength()); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", details...)
		return
	}

	if err := h.authService.ResetPassword(r.Context(), req.Token, req.Password); err != nil {
		switch {
		case errors.Is(err, auth.ErrPasswordResetTokenInvalid):
			response.WriteError(w, r, http.StatusBadRequest, "invalid_reset_token", "password reset token is invalid or expired")
		default:
			response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to reset password")
		}
		return
	}

	response.WriteJSON(w, r, http.StatusOK, openapi.StatusResponse{Status: openapi.StatusResponseStatusPasswordChanged})
}

// VKStart начинает внешний OAuth-flow и перенаправляет пользователя на страницу VK ID.
func (h AuthHandler) VKStart(w http.ResponseWriter, r *http.Request) {
	redirectURL, err := h.authService.StartVKOAuth(r.Context())
	if err != nil {
		if errors.Is(err, auth.ErrOAuthNotConfigured) {
			response.WriteError(w, r, http.StatusNotImplemented, "oauth_not_configured", "vk oauth is not configured yet")
			return
		}

		response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to start oauth flow")
		return
	}

	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// VKCallback завершает внешний OAuth-flow и создаёт обычную локальную сессию backend.
func (h AuthHandler) VKCallback(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || state == "" {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "code and state query params are required")
		return
	}

	result, err := h.authService.FinishVKOAuth(r.Context(), code, state, sessionMetadataFromRequest(r))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrOAuthNotConfigured):
			response.WriteError(w, r, http.StatusNotImplemented, "oauth_not_configured", "vk oauth is not configured yet")
		case errors.Is(err, auth.ErrInvalidOAuthState):
			response.WriteError(w, r, http.StatusBadRequest, "invalid_oauth_state", "oauth state is invalid or expired")
		case errors.Is(err, auth.ErrOAuthEmailMissing):
			response.WriteError(w, r, http.StatusBadRequest, "oauth_email_missing", "vk account did not provide email")
		default:
			response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to finish oauth flow")
		}
		return
	}

	response.WriteJSON(w, r, http.StatusOK, toAuthResponse(result))
}

// YandexStart начинает внешний OAuth-flow и перенаправляет пользователя на страницу Яндекс ID.
func (h AuthHandler) YandexStart(w http.ResponseWriter, r *http.Request) {
	redirectURL, err := h.authService.StartYandexOAuth(r.Context())
	if err != nil {
		if errors.Is(err, auth.ErrOAuthNotConfigured) {
			response.WriteError(w, r, http.StatusNotImplemented, "oauth_not_configured", "yandex oauth is not configured yet")
			return
		}

		response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to start oauth flow")
		return
	}

	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// YandexCallback завершает внешний OAuth-flow Яндекс ID и создаёт обычную локальную сессию backend.
func (h AuthHandler) YandexCallback(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || state == "" {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "code and state query params are required")
		return
	}

	result, err := h.authService.FinishYandexOAuth(r.Context(), code, state, sessionMetadataFromRequest(r))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrOAuthNotConfigured):
			response.WriteError(w, r, http.StatusNotImplemented, "oauth_not_configured", "yandex oauth is not configured yet")
		case errors.Is(err, auth.ErrInvalidOAuthState):
			response.WriteError(w, r, http.StatusBadRequest, "invalid_oauth_state", "oauth state is invalid or expired")
		case errors.Is(err, auth.ErrOAuthEmailMissing):
			response.WriteError(w, r, http.StatusBadRequest, "oauth_email_missing", "yandex account did not provide email")
		default:
			response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to finish oauth flow")
		}
		return
	}

	response.WriteJSON(w, r, http.StatusOK, toAuthResponse(result))
}

func validateRegisterRequest(req openapi.RegisterRequest, passwordMinLength int) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	emailStr := strings.TrimSpace(string(req.Email))
	if emailStr == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("email"), Message: "field is required"})
	} else if !isValidEmail(emailStr) {
		details = append(details, openapi.ErrorDetail{Field: strPtr("email"), Message: "must be a valid email"})
	}

	if strings.TrimSpace(req.Password) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("password"), Message: "field is required"})
	} else if len(req.Password) < passwordMinLength {
		details = append(details, openapi.ErrorDetail{
			Field:   strPtr("password"),
			Message: fmt.Sprintf("must be at least %d characters", passwordMinLength),
		})
	}

	return details
}

func validateLoginRequest(req openapi.LoginRequest) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	emailStr := strings.TrimSpace(string(req.Email))
	if emailStr == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("email"), Message: "field is required"})
	} else if !isValidEmail(emailStr) {
		details = append(details, openapi.ErrorDetail{Field: strPtr("email"), Message: "must be a valid email"})
	}

	if strings.TrimSpace(req.Password) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("password"), Message: "field is required"})
	}

	return details
}

func validateForgotPasswordRequest(req openapi.ForgotPasswordRequest) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	emailStr := strings.TrimSpace(string(req.Email))
	if emailStr == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("email"), Message: "field is required"})
	} else if !isValidEmail(emailStr) {
		details = append(details, openapi.ErrorDetail{Field: strPtr("email"), Message: "must be a valid email"})
	}

	return details
}

func validateResetPasswordRequest(req openapi.ResetPasswordRequest, passwordMinLength int) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	if strings.TrimSpace(req.Token) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("token"), Message: "field is required"})
	}

	if strings.TrimSpace(req.Password) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("password"), Message: "field is required"})
	} else if len(req.Password) < passwordMinLength {
		details = append(details, openapi.ErrorDetail{
			Field:   strPtr("password"),
			Message: fmt.Sprintf("must be at least %d characters", passwordMinLength),
		})
	}

	return details
}

func validateTelegramLoginRequest(req openapi.TelegramLoginRequest) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	if req.Id <= 0 {
		details = append(details, openapi.ErrorDetail{Field: strPtr("id"), Message: "must be greater than zero"})
	}

	if req.AuthDate <= 0 {
		details = append(details, openapi.ErrorDetail{Field: strPtr("auth_date"), Message: "must be a valid unix timestamp"})
	}

	if strings.TrimSpace(req.Hash) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("hash"), Message: "field is required"})
	}

	return details
}

// toAuthResponse конвертирует доменный AuthResult в openapi.AuthResponse для HTTP-ответа.
func toAuthResponse(result auth.AuthResult) openapi.AuthResponse {
	return openapi.AuthResponse{
		User:    authUserToAPI(result.User),
		Session: openapi.AuthSession{
			AccessToken: result.Session.AccessToken,
			TokenType:   "Bearer",
			ExpiresAt:   result.Session.ExpiresAt,
		},
	}
}

// authUserToAPI конвертирует доменного auth.User в openapi.AuthUser.
// auth.User.ID — строковый UUID, openapi.AuthUser.Id — типизированный openapi_types.UUID.
func authUserToAPI(user auth.User) openapi.AuthUser {
	email := openapi_types.Email(user.Email)

	apiUser := openapi.AuthUser{
		Id:    mustParseUUID(user.ID),
		Email: &email,
	}
	if user.Name != "" {
		apiUser.Name = &user.Name
	}

	return apiUser
}

