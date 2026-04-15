package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	openapi "kartochki-online-backend/api/gen"
	"kartochki-online-backend/internal/auth"
	"kartochki-online-backend/internal/http/authctx"
	"kartochki-online-backend/internal/http/response"
)

// AuthHandler обслуживает публичные auth-сценарии и маршруты текущего пользователя.
type AuthHandler struct {
	authService       *auth.Service
	secureCookie      bool   // true в production: кука отправляется только по HTTPS
	frontendURLOrigin string // ожидаемый origin frontend для OAuth redirect_uri
}

// NewAuthHandler создаёт обработчик auth endpoint.
// secureCookie должен быть true в production-окружении.
func NewAuthHandler(authService *auth.Service, secureCookie bool, frontendURL string) AuthHandler {
	return AuthHandler{
		authService:       authService,
		secureCookie:      secureCookie,
		frontendURLOrigin: normalizeOrigin(frontendURL),
	}
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

	setAuthCookie(w, result.Session.AccessToken, h.secureCookie)
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

	setAuthCookie(w, result.Session.AccessToken, h.secureCookie)
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

	setAuthCookie(w, result.Session.AccessToken, h.secureCookie)
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

	clearAuthCookie(w, h.secureCookie)
	response.WriteJSON(w, r, http.StatusOK, openapi.StatusResponse{Status: openapi.StatusResponseStatusLoggedOut})
}

// Me возвращает текущего пользователя, который уже был загружен middleware.
func (h AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUserFromCtx(w, r)
	if !ok {
		return
	}

	user = h.authService.WithLatestOAuthAvatar(r.Context(), user)
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

// VKWidget завершает вход через VK ID One Tap и создаёт обычную локальную сессию backend.
func (h AuthHandler) VKWidget(w http.ResponseWriter, r *http.Request) {
	var req openapi.VkWidgetLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_widget_payload", "request body must be valid JSON")
		return
	}

	if details := h.validateVKWidgetRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_widget_payload", "vk widget payload is invalid", details...)
		return
	}

	result, err := h.authService.LoginWithVKWidget(r.Context(), auth.VKWidgetLoginInput{
		Code:         req.Code,
		DeviceID:     req.DeviceId,
		CodeVerifier: req.CodeVerifier,
		RedirectURI:  req.RedirectUri,
	}, sessionMetadataFromRequest(r))
	if err != nil {
		if h.writeWidgetOAuthError(w, r, err) {
			return
		}
		response.WriteError(w, r, http.StatusInternalServerError, "oauth_provider_error", "failed to login with vk widget")
		return
	}

	setAuthCookie(w, result.Session.AccessToken, h.secureCookie)
	response.WriteJSON(w, r, http.StatusOK, toAuthResponse(result))
}

// YandexWidget завершает вход по access token от виджета Яндекс ID и создаёт локальную сессию backend.
func (h AuthHandler) YandexWidget(w http.ResponseWriter, r *http.Request) {
	var req openapi.YandexWidgetLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_widget_payload", "request body must be valid JSON")
		return
	}

	if details := validateYandexWidgetRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_widget_payload", "yandex widget payload is invalid", details...)
		return
	}

	result, err := h.authService.LoginWithYandexWidget(r.Context(), req.AccessToken, sessionMetadataFromRequest(r))
	if err != nil {
		if h.writeWidgetOAuthError(w, r, err) {
			return
		}
		response.WriteError(w, r, http.StatusInternalServerError, "oauth_provider_error", "failed to login with yandex widget")
		return
	}

	setAuthCookie(w, result.Session.AccessToken, h.secureCookie)
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

func (h AuthHandler) validateVKWidgetRequest(req openapi.VkWidgetLoginRequest) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	if strings.TrimSpace(req.Code) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("code"), Message: "field is required"})
	}
	if strings.TrimSpace(req.DeviceId) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("device_id"), Message: "field is required"})
	}
	if strings.TrimSpace(req.CodeVerifier) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("code_verifier"), Message: "field is required"})
	} else if !isValidPKCEVerifier(req.CodeVerifier) {
		details = append(details, openapi.ErrorDetail{Field: strPtr("code_verifier"), Message: "must be a valid PKCE verifier"})
	}
	if strings.TrimSpace(req.RedirectUri) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("redirect_uri"), Message: "field is required"})
	} else if !isValidVKRedirectURI(req.RedirectUri, h.frontendURLOrigin) {
		details = append(details, openapi.ErrorDetail{Field: strPtr("redirect_uri"), Message: "must be a valid /auth redirect uri"})
	}

	return details
}

// isValidPKCEVerifier проверяет формат verifier до запроса к VK, чтобы отсечь явно чужой или повреждённый payload.
func isValidPKCEVerifier(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 43 || len(value) > 128 {
		return false
	}

	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}
		if r == '-' || r == '.' || r == '_' || r == '~' {
			continue
		}
		return false
	}

	return true
}

// isValidVKRedirectURI не даёт использовать backend для обмена code, выпущенного под неожиданный redirect URL.
func isValidVKRedirectURI(value string, expectedOrigin string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}

	if parsed.Host == "" || parsed.Path != "/auth" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}

	origin := normalizeOrigin(parsed.Scheme + "://" + parsed.Host)
	if expectedOrigin != "" && origin == expectedOrigin {
		return true
	}

	// Локальный http разрешён только когда сам backend настроен на локальный frontend.
	return strings.HasPrefix(expectedOrigin, "http://localhost:") &&
		parsed.Scheme == "http" &&
		strings.HasPrefix(parsed.Host, "localhost:")
}

func normalizeOrigin(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}

	return parsed.Scheme + "://" + parsed.Host
}

func validateYandexWidgetRequest(req openapi.YandexWidgetLoginRequest) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	if strings.TrimSpace(req.AccessToken) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("access_token"), Message: "field is required"})
	}

	return details
}

func (h AuthHandler) writeWidgetOAuthError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, auth.ErrOAuthNotConfigured):
		response.WriteError(w, r, http.StatusNotImplemented, "provider_not_configured", "oauth provider is not configured")
	case errors.Is(err, auth.ErrOAuthTokenInvalid):
		response.WriteError(w, r, http.StatusUnauthorized, "provider_auth_failed", "provider rejected widget payload")
	case errors.Is(err, auth.ErrOAuthProviderError):
		response.WriteError(w, r, http.StatusInternalServerError, "oauth_provider_error", "oauth provider returned an unexpected response")
	default:
		return false
	}

	return true
}

// toAuthResponse конвертирует доменный AuthResult в openapi.AuthResponse для HTTP-ответа.
func toAuthResponse(result auth.AuthResult) openapi.AuthResponse {
	return openapi.AuthResponse{
		User: authUserToAPI(result.User),
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
	apiUser := openapi.AuthUser{
		Id: mustParseUUID(user.ID),
	}
	if user.Email != "" {
		// Email в контракте nullable: OAuth-провайдеры могут не вернуть почту.
		email := openapi_types.Email(user.Email)
		apiUser.Email = &email
	}
	if user.Name != "" {
		apiUser.Name = &user.Name
	}
	if user.AvatarURL != "" {
		// Аватар может прийти только от OAuth-провайдера, поэтому поле остаётся nullable.
		apiUser.AvatarUrl = &user.AvatarURL
	}

	return apiUser
}
