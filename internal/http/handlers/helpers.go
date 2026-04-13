package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"strings"

	"github.com/google/uuid"

	"kartochki-online-backend/internal/auth"
	"kartochki-online-backend/internal/http/authctx"
	"kartochki-online-backend/internal/http/response"
)

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
