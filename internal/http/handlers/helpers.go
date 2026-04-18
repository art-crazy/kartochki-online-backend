package handlers

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"kartochki-online-backend/internal/auth"
	"kartochki-online-backend/internal/http/authctx"
	"kartochki-online-backend/internal/http/response"
)

// CookieConfig описывает, как HTTP-слой должен выставлять auth cookie.
type CookieConfig struct {
	Domain string
	Secure bool
}

// strPtr возвращает указатель на строку — нужен для заполнения openapi.ErrorDetail.Field,
// которое является *string из-за omitempty в OpenAPI-схеме.
func strPtr(s string) *string {
	return &s
}

// stringOrEmpty безопасно разыменовывает *string, возвращая пустую строку для nil.
func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// mustParseUUID парсит строковый UUID из доменного слоя в типизированный uuid.UUID.
// Домен всегда хранит валидные UUID-строки (сгенерированы базой), поэтому ошибка
// при парсинге означает несоответствие данных — возвращаем нулевой UUID как fallback.
func mustParseUUID(s string) uuid.UUID {
	id, _ := uuid.Parse(s)
	return id
}

// currentUserFromCtx извлекает auth.User из контекста запроса.
// Если middleware не положил пользователя — пишет 401 и возвращает false.
func currentUserFromCtx(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	user, ok := authctx.User(r.Context())
	if !ok {
		response.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "authorization token is invalid")
	}
	return user, ok
}

// isValidEmail проверяет, является ли строка корректным email-адресом.
// Использует стандартную библиотеку mail — строгая, но достаточная для transport-слоя.
func isValidEmail(value string) bool {
	value = strings.TrimSpace(value)
	parsed, err := mail.ParseAddress(value)
	if err != nil {
		return false
	}
	return parsed.Address == value
}

// setAuthCookie устанавливает HttpOnly-куку с токеном сессии.
// expiresAt задаёт срок действия куки — браузер удалит её после этого времени.
// Domain нужен для общей авторизации между поддоменами, а Secure включаем только там, где cookie реально
// может вернуться обратно по HTTPS.
func setAuthCookie(w http.ResponseWriter, token string, expiresAt time.Time, cfg CookieConfig) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge <= 0 {
		maxAge = 0
	}
	cookie := &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   cfg.Secure,
		MaxAge:   maxAge,
	}
	if cfg.Domain != "" {
		cookie.Domain = cfg.Domain
	}
	http.SetCookie(w, cookie)
}

// clearAuthCookie сбрасывает куку auth_token при logout.
// Max-Age=0 — стандартный способ удаления куки по RFC 6265.
// При удалении повторяем тот же Domain, иначе браузер не удалит установленную cookie.
func clearAuthCookie(w http.ResponseWriter, cfg CookieConfig) {
	cookie := &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   cfg.Secure,
		MaxAge:   0,
	}
	if cfg.Domain != "" {
		cookie.Domain = cfg.Domain
	}
	http.SetCookie(w, cookie)
}

// decodeJSON десериализует JSON-тело запроса в dst.
// Запрещает неизвестные поля и несколько JSON-значений подряд.
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

// clientIPAddress извлекает IP без порта после chi.RealIP middleware.
func clientIPAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}

	return strings.TrimSpace(r.RemoteAddr)
}
