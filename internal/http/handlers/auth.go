package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"

	"kartochki-online-backend/internal/auth"
	"kartochki-online-backend/internal/http/authctx"
	"kartochki-online-backend/internal/http/contracts"
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
	var req contracts.RegisterRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if details := validateRegisterRequest(req, h.authService.PasswordMinLength()); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", details...)
		return
	}

	result, err := h.authService.Register(r.Context(), auth.RegisterInput{
		Name:     req.Name,
		Email:    req.Email,
		Password: req.Password,
	})
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
				contracts.ErrorDetail{
					Field:   "password",
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
	var req contracts.LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if details := validateLoginRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", details...)
		return
	}

	result, err := h.authService.Login(r.Context(), auth.LoginInput{
		Email:    req.Email,
		Password: req.Password,
	})
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
	var req contracts.TelegramLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if details := validateTelegramLoginRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", details...)
		return
	}

	result, err := h.authService.LoginWithTelegram(r.Context(), auth.TelegramLoginData{
		ID:        req.ID,
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Username:  req.Username,
		PhotoURL:  req.PhotoURL,
		AuthDate:  req.AuthDate,
		Hash:      req.Hash,
	})
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

	response.WriteJSON(w, r, http.StatusOK, contracts.LogoutResponse{Status: "logged_out"})
}

// Me возвращает текущего пользователя, который уже был загружен middleware.
func (h AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := authctx.User(r.Context())
	if !ok {
		response.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "authorization token is invalid")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, contracts.CurrentUserResponse{
		User: contracts.AuthUser{
			ID:    user.ID,
			Name:  user.Name,
			Email: user.Email,
		},
	})
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

	result, err := h.authService.FinishVKOAuth(r.Context(), code, state)
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

	result, err := h.authService.FinishYandexOAuth(r.Context(), code, state)
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

func decodeJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return err
	}

	if decoder.More() {
		return fmt.Errorf("multiple JSON values are not allowed")
	}

	return nil
}

func validateRegisterRequest(req contracts.RegisterRequest, passwordMinLength int) []contracts.ErrorDetail {
	var details []contracts.ErrorDetail

	if strings.TrimSpace(req.Email) == "" {
		details = append(details, contracts.ErrorDetail{Field: "email", Message: "field is required"})
	} else if !isLikelyEmail(req.Email) {
		details = append(details, contracts.ErrorDetail{Field: "email", Message: "must be a valid email"})
	}

	if strings.TrimSpace(req.Password) == "" {
		details = append(details, contracts.ErrorDetail{Field: "password", Message: "field is required"})
	} else if len(req.Password) < passwordMinLength {
		details = append(details, contracts.ErrorDetail{
			Field:   "password",
			Message: fmt.Sprintf("must be at least %d characters", passwordMinLength),
		})
	}

	return details
}

func validateLoginRequest(req contracts.LoginRequest) []contracts.ErrorDetail {
	var details []contracts.ErrorDetail

	if strings.TrimSpace(req.Email) == "" {
		details = append(details, contracts.ErrorDetail{Field: "email", Message: "field is required"})
	} else if !isLikelyEmail(req.Email) {
		details = append(details, contracts.ErrorDetail{Field: "email", Message: "must be a valid email"})
	}

	if strings.TrimSpace(req.Password) == "" {
		details = append(details, contracts.ErrorDetail{Field: "password", Message: "field is required"})
	}

	return details
}

func validateTelegramLoginRequest(req contracts.TelegramLoginRequest) []contracts.ErrorDetail {
	var details []contracts.ErrorDetail

	if req.ID <= 0 {
		details = append(details, contracts.ErrorDetail{Field: "id", Message: "must be greater than zero"})
	}

	if req.AuthDate <= 0 {
		details = append(details, contracts.ErrorDetail{Field: "auth_date", Message: "must be a valid unix timestamp"})
	}

	if strings.TrimSpace(req.Hash) == "" {
		details = append(details, contracts.ErrorDetail{Field: "hash", Message: "field is required"})
	}

	return details
}

func toAuthResponse(result auth.AuthResult) contracts.AuthResponse {
	return contracts.AuthResponse{
		User: contracts.AuthUser{
			ID:    result.User.ID,
			Name:  result.User.Name,
			Email: result.User.Email,
		},
		Session: contracts.AuthSession{
			AccessToken: result.Session.AccessToken,
			TokenType:   "Bearer",
			ExpiresAt:   result.Session.ExpiresAt,
		},
	}
}

func isLikelyEmail(value string) bool {
	email := strings.TrimSpace(value)
	parsed, err := mail.ParseAddress(email)
	if err != nil {
		return false
	}

	return parsed.Address == email
}
