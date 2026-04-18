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
	authService      *auth.Service
	authCookieConfig CookieConfig
}

// NewAuthHandler создаёт обработчик auth endpoint.
func NewAuthHandler(authService *auth.Service, authCookieConfig CookieConfig) AuthHandler {
	return AuthHandler{
		authService:      authService,
		authCookieConfig: authCookieConfig,
	}
}

// Register запускает двухшаговую регистрацию и отправляет код подтверждения на email.
func (h AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req openapi.RegisterRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "Тело запроса должно быть корректным JSON.")
		return
	}

	if details := validateRegisterRequest(req, h.authService.PasswordMinLength()); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "Запрос содержит некорректные данные.", details...)
		return
	}

	result, err := h.authService.Register(r.Context(), auth.RegisterInput{
		Name:      stringOrEmpty(req.Name),
		Email:     string(req.Email),
		Password:  req.Password,
		IPAddress: clientIPAddress(r),
	})
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrEmailAlreadyExists):
			response.WriteError(w, r, http.StatusConflict, "email_taken", "Пользователь с таким email уже зарегистрирован.")
		case errors.Is(err, auth.ErrPasswordTooShort):
			response.WriteError(
				w,
				r,
				http.StatusBadRequest,
				"validation_error",
				"Запрос содержит некорректные данные.",
				openapi.ErrorDetail{
					Field:   strPtr("password"),
					Message: fmt.Sprintf("Пароль должен содержать не менее %d символов.", h.authService.PasswordMinLength()),
				},
			)
		case errors.Is(err, auth.ErrRegistrationRateLimited):
			response.WriteError(w, r, http.StatusTooManyRequests, "registration_rate_limited", "Слишком много попыток регистрации. Попробуйте позже.")
		default:
			response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Не удалось начать регистрацию.")
		}
		return
	}

	response.WriteJSON(w, r, http.StatusCreated, openapi.RegisterPendingResponse{
		Status:                   result.Status,
		VerificationId:           mustParseUUID(result.VerificationID),
		Email:                    openapi_types.Email(result.Email),
		CodeLength:               result.CodeLength,
		ResendAvailableInSeconds: result.ResendAvailableInSeconds,
		ExpiresInSeconds:         result.ExpiresInSeconds,
	})
}

// Login создаёт новую сессию по корректной паре email и пароль.
func (h AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req openapi.LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "Тело запроса должно быть корректным JSON.")
		return
	}

	if details := validateLoginRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "Запрос содержит некорректные данные.", details...)
		return
	}

	result, err := h.authService.Login(r.Context(), auth.LoginInput{
		Email:    string(req.Email),
		Password: req.Password,
	}, sessionMetadataFromRequest(r))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			response.WriteError(w, r, http.StatusUnauthorized, "invalid_credentials", "Неверный email или пароль.")
		default:
			response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Не удалось выполнить вход.")
		}
		return
	}

	setAuthCookie(w, result.Session.AccessToken, result.Session.ExpiresAt, h.authCookieConfig)
	response.WriteJSON(w, r, http.StatusOK, toAuthResponse(result))
}

// TelegramLogin завершает вход через Telegram Login Widget и создаёт локальную сессию.
func (h AuthHandler) TelegramLogin(w http.ResponseWriter, r *http.Request) {
	var req openapi.TelegramLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "Тело запроса должно быть корректным JSON.")
		return
	}

	if details := validateTelegramLoginRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "Запрос содержит некорректные данные.", details...)
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
			response.WriteError(w, r, http.StatusNotImplemented, "telegram_not_configured", "Вход через Telegram ещё не настроен.")
		case errors.Is(err, auth.ErrTelegramAuthInvalid):
			response.WriteError(w, r, http.StatusBadRequest, "invalid_telegram_auth", "Данные авторизации Telegram некорректны.")
		case errors.Is(err, auth.ErrTelegramAuthExpired):
			response.WriteError(w, r, http.StatusBadRequest, "telegram_auth_expired", "Срок действия данных авторизации Telegram истёк.")
		default:
			response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Не удалось выполнить вход через Telegram.")
		}
		return
	}

	setAuthCookie(w, result.Session.AccessToken, result.Session.ExpiresAt, h.authCookieConfig)
	response.WriteJSON(w, r, http.StatusOK, toAuthResponse(result))
}

// Logout отзывает текущую сессию по Bearer-токену.
func (h AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	token, ok := authctx.AccessToken(r.Context())
	if !ok {
		response.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Требуется токен авторизации.")
		return
	}

	if err := h.authService.Logout(r.Context(), token); err != nil {
		switch {
		case errors.Is(err, auth.ErrUnauthorized):
			response.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Токен авторизации недействителен.")
		default:
			response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Не удалось завершить сеанс.")
		}
		return
	}

	clearAuthCookie(w, h.authCookieConfig)
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
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "Тело запроса должно быть корректным JSON.")
		return
	}

	if details := validateForgotPasswordRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "Запрос содержит некорректные данные.", details...)
		return
	}

	if err := h.authService.ForgotPassword(r.Context(), string(req.Email)); err != nil {
		response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Не удалось обработать запрос на сброс пароля.")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, openapi.StatusResponse{Status: openapi.StatusResponseStatusAccepted})
}

// ResetPassword принимает токен из письма и новый пароль, затем обновляет пароль пользователя.
func (h AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req openapi.ResetPasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "Тело запроса должно быть корректным JSON.")
		return
	}

	if details := validateResetPasswordRequest(req, h.authService.PasswordMinLength()); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "Запрос содержит некорректные данные.", details...)
		return
	}

	if err := h.authService.ResetPassword(r.Context(), req.Token, req.Password); err != nil {
		switch {
		case errors.Is(err, auth.ErrPasswordResetTokenInvalid):
			response.WriteError(w, r, http.StatusBadRequest, "invalid_reset_token", "Токен сброса пароля недействителен или срок его действия истёк.")
		default:
			response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Не удалось сбросить пароль.")
		}
		return
	}

	response.WriteJSON(w, r, http.StatusOK, openapi.StatusResponse{Status: openapi.StatusResponseStatusPasswordChanged})
}

// VKWidget завершает вход через VK ID One Tap и создаёт обычную локальную сессию backend.
func (h AuthHandler) VKWidget(w http.ResponseWriter, r *http.Request) {
	var req openapi.VkWidgetLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_widget_payload", "Тело запроса должно быть корректным JSON.")
		return
	}

	if details := h.validateVKWidgetRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_widget_payload", "Данные виджета VK некорректны.", details...)
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
		response.WriteError(w, r, http.StatusInternalServerError, "oauth_provider_error", "Не удалось выполнить вход через виджет VK.")
		return
	}

	setAuthCookie(w, result.Session.AccessToken, result.Session.ExpiresAt, h.authCookieConfig)
	response.WriteJSON(w, r, http.StatusOK, toAuthResponse(result))
}

// VKOAuth завершает стандартный VK OAuth 2.0 Authorization Code + PKCE flow и создаёт локальную сессию backend.
func (h AuthHandler) VKOAuth(w http.ResponseWriter, r *http.Request) {
	var req openapi.VkOAuthLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "Тело запроса должно быть корректным JSON.")
		return
	}

	if details := h.validateVKOAuthRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_widget_payload", "Данные OAuth VK некорректны.", details...)
		return
	}

	result, err := h.authService.LoginWithVKOAuth(r.Context(), auth.VKOAuthLoginInput{
		Code:         req.Code,
		DeviceID:     req.DeviceId,
		CodeVerifier: req.CodeVerifier,
		RedirectURI:  req.RedirectUri,
	}, sessionMetadataFromRequest(r))
	if err != nil {
		if h.writeWidgetOAuthError(w, r, err) {
			return
		}
		response.WriteError(w, r, http.StatusInternalServerError, "oauth_provider_error", "Не удалось выполнить вход через OAuth VK.")
		return
	}

	setAuthCookie(w, result.Session.AccessToken, result.Session.ExpiresAt, h.authCookieConfig)
	response.WriteJSON(w, r, http.StatusOK, toAuthResponse(result))
}

// YandexWidget завершает вход по access token от виджета Яндекс ID и создаёт локальную сессию backend.
func (h AuthHandler) YandexWidget(w http.ResponseWriter, r *http.Request) {
	var req openapi.YandexWidgetLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_widget_payload", "Тело запроса должно быть корректным JSON.")
		return
	}

	if details := validateYandexWidgetRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_widget_payload", "Данные виджета Яндекс ID некорректны.", details...)
		return
	}

	result, err := h.authService.LoginWithYandexWidget(r.Context(), req.AccessToken, sessionMetadataFromRequest(r))
	if err != nil {
		if h.writeWidgetOAuthError(w, r, err) {
			return
		}
		response.WriteError(w, r, http.StatusInternalServerError, "oauth_provider_error", "Не удалось выполнить вход через виджет Яндекс ID.")
		return
	}

	setAuthCookie(w, result.Session.AccessToken, result.Session.ExpiresAt, h.authCookieConfig)
	response.WriteJSON(w, r, http.StatusOK, toAuthResponse(result))
}

func validateRegisterRequest(req openapi.RegisterRequest, passwordMinLength int) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	emailStr := strings.TrimSpace(string(req.Email))
	if emailStr == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("email"), Message: "Поле обязательно для заполнения."})
	} else if !isValidEmail(emailStr) {
		details = append(details, openapi.ErrorDetail{Field: strPtr("email"), Message: "Укажите корректный email."})
	}

	if strings.TrimSpace(req.Password) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("password"), Message: "Поле обязательно для заполнения."})
	} else if len(req.Password) < passwordMinLength {
		details = append(details, openapi.ErrorDetail{
			Field:   strPtr("password"),
			Message: fmt.Sprintf("Пароль должен содержать не менее %d символов.", passwordMinLength),
		})
	}

	return details
}

func validateLoginRequest(req openapi.LoginRequest) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	emailStr := strings.TrimSpace(string(req.Email))
	if emailStr == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("email"), Message: "Поле обязательно для заполнения."})
	} else if !isValidEmail(emailStr) {
		details = append(details, openapi.ErrorDetail{Field: strPtr("email"), Message: "Укажите корректный email."})
	}

	if strings.TrimSpace(req.Password) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("password"), Message: "Поле обязательно для заполнения."})
	}

	return details
}

func validateForgotPasswordRequest(req openapi.ForgotPasswordRequest) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	emailStr := strings.TrimSpace(string(req.Email))
	if emailStr == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("email"), Message: "Поле обязательно для заполнения."})
	} else if !isValidEmail(emailStr) {
		details = append(details, openapi.ErrorDetail{Field: strPtr("email"), Message: "Укажите корректный email."})
	}

	return details
}

func validateResetPasswordRequest(req openapi.ResetPasswordRequest, passwordMinLength int) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	if strings.TrimSpace(req.Token) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("token"), Message: "Поле обязательно для заполнения."})
	}

	if strings.TrimSpace(req.Password) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("password"), Message: "Поле обязательно для заполнения."})
	} else if len(req.Password) < passwordMinLength {
		details = append(details, openapi.ErrorDetail{
			Field:   strPtr("password"),
			Message: fmt.Sprintf("Пароль должен содержать не менее %d символов.", passwordMinLength),
		})
	}

	return details
}

func validateTelegramLoginRequest(req openapi.TelegramLoginRequest) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	if req.Id <= 0 {
		details = append(details, openapi.ErrorDetail{Field: strPtr("id"), Message: "Значение должно быть больше нуля."})
	}

	if req.AuthDate <= 0 {
		details = append(details, openapi.ErrorDetail{Field: strPtr("auth_date"), Message: "Укажите корректный Unix timestamp."})
	}

	if strings.TrimSpace(req.Hash) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("hash"), Message: "Поле обязательно для заполнения."})
	}

	return details
}

func (h AuthHandler) validateVKWidgetRequest(req openapi.VkWidgetLoginRequest) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	if strings.TrimSpace(req.Code) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("code"), Message: "Поле обязательно для заполнения."})
	}
	if strings.TrimSpace(req.DeviceId) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("device_id"), Message: "Поле обязательно для заполнения."})
	}
	if strings.TrimSpace(req.CodeVerifier) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("code_verifier"), Message: "Поле обязательно для заполнения."})
	} else if !isValidPKCEVerifier(req.CodeVerifier) {
		details = append(details, openapi.ErrorDetail{Field: strPtr("code_verifier"), Message: "Укажите корректный PKCE verifier."})
	}
	if strings.TrimSpace(req.RedirectUri) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("redirect_uri"), Message: "Поле обязательно для заполнения."})
	}

	return details
}

func (h AuthHandler) validateVKOAuthRequest(req openapi.VkOAuthLoginRequest) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	if strings.TrimSpace(req.Code) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("code"), Message: "Поле обязательно для заполнения."})
	}
	if strings.TrimSpace(req.DeviceId) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("device_id"), Message: "Поле обязательно для заполнения."})
	}
	if strings.TrimSpace(req.CodeVerifier) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("code_verifier"), Message: "Поле обязательно для заполнения."})
	} else if !isValidPKCEVerifier(req.CodeVerifier) {
		details = append(details, openapi.ErrorDetail{Field: strPtr("code_verifier"), Message: "Укажите корректный PKCE verifier."})
	}
	if strings.TrimSpace(req.RedirectUri) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("redirect_uri"), Message: "Поле обязательно для заполнения."})
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

func validateYandexWidgetRequest(req openapi.YandexWidgetLoginRequest) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	if strings.TrimSpace(req.AccessToken) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("access_token"), Message: "Поле обязательно для заполнения."})
	}

	return details
}

func (h AuthHandler) writeWidgetOAuthError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, auth.ErrOAuthNotConfigured):
		response.WriteError(w, r, http.StatusNotImplemented, "provider_not_configured", "Провайдер OAuth не настроен.")
	case errors.Is(err, auth.ErrOAuthTokenInvalid):
		response.WriteError(w, r, http.StatusUnauthorized, "provider_auth_failed", "Провайдер отклонил данные авторизации.")
	case errors.Is(err, auth.ErrOAuthProviderError):
		response.WriteError(w, r, http.StatusInternalServerError, "oauth_provider_error", "Провайдер OAuth вернул непредвиденный ответ.")
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
