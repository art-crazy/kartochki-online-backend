package handlers

import (
	"errors"
	"net/http"
	"strings"
	"unicode"

	openapi_types "github.com/oapi-codegen/runtime/types"

	openapi "kartochki-online-backend/api/gen"
	"kartochki-online-backend/internal/auth"
	"kartochki-online-backend/internal/http/response"
)

// VerifyRegister завершает регистрацию после ввода одноразового кода из письма.
func (h AuthHandler) VerifyRegister(w http.ResponseWriter, r *http.Request) {
	var req openapi.VerifyRegisterRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "Тело запроса должно быть корректным JSON.")
		return
	}

	if details := validateVerifyRegisterRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "Запрос содержит некорректные данные.", details...)
		return
	}

	result, err := h.authService.VerifyRegistration(r.Context(), auth.VerifyRegistrationInput{
		VerificationID: req.VerificationId.String(),
		Code:           req.Code,
	}, sessionMetadataFromRequest(r))
	if err != nil {
		writeRegisterVerificationError(w, r, err)
		return
	}

	setAuthCookie(w, result.Session.AccessToken, result.Session.ExpiresAt, h.authCookieDomain)
	response.WriteJSON(w, r, http.StatusOK, toAuthResponse(result))
}

// ResendRegisterCode выпускает новый код и повторно отправляет письмо для активной регистрации.
func (h AuthHandler) ResendRegisterCode(w http.ResponseWriter, r *http.Request) {
	var req openapi.ResendRegisterCodeRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "Тело запроса должно быть корректным JSON.")
		return
	}

	if details := validateResendRegisterCodeRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "Запрос содержит некорректные данные.", details...)
		return
	}

	result, err := h.authService.ResendRegistrationCode(r.Context(), auth.ResendRegistrationCodeInput{
		VerificationID: req.VerificationId.String(),
	})
	if err != nil {
		writeRegisterResendError(w, r, err)
		return
	}

	response.WriteJSON(w, r, http.StatusOK, openapi.RegisterPendingResponse{
		Status:                   "code_resent",
		VerificationId:           mustParseUUID(result.VerificationID),
		Email:                    openapi_types.Email(result.Email),
		CodeLength:               result.CodeLength,
		ResendAvailableInSeconds: result.ResendAvailableInSeconds,
		ExpiresInSeconds:         result.ExpiresInSeconds,
	})
}

func validateVerifyRegisterRequest(req openapi.VerifyRegisterRequest) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	if req.VerificationId.String() == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("verification_id"), Message: "Поле обязательно для заполнения."})
	}

	code := strings.TrimSpace(req.Code)
	if code == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("code"), Message: "Поле обязательно для заполнения."})
	} else if !isSixDigitCode(code) {
		details = append(details, openapi.ErrorDetail{Field: strPtr("code"), Message: "Код должен состоять ровно из 6 цифр."})
	}

	return details
}

func validateResendRegisterCodeRequest(req openapi.ResendRegisterCodeRequest) []openapi.ErrorDetail {
	if req.VerificationId.String() == "" {
		return []openapi.ErrorDetail{{Field: strPtr("verification_id"), Message: "Поле обязательно для заполнения."}}
	}
	return nil
}

func isSixDigitCode(value string) bool {
	if len(value) != 6 {
		return false
	}

	for _, r := range value {
		if !unicode.IsDigit(r) {
			return false
		}
	}

	return true
}

func writeRegisterVerificationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrVerificationNotFound):
		response.WriteError(w, r, http.StatusNotFound, "verification_not_found", "Регистрация с указанным кодом подтверждения не найдена.")
	case errors.Is(err, auth.ErrVerificationCodeInvalid):
		response.WriteError(w, r, http.StatusUnauthorized, "invalid_verification_code", "Указан неверный код подтверждения.")
	case errors.Is(err, auth.ErrVerificationCodeExpired):
		response.WriteError(w, r, http.StatusGone, "verification_expired", "Срок действия кода подтверждения истёк.")
	case errors.Is(err, auth.ErrVerificationAttemptsExceeded):
		response.WriteError(w, r, http.StatusTooManyRequests, "verification_attempts_exceeded", "Превышено допустимое количество попыток подтверждения.")
	case errors.Is(err, auth.ErrVerificationAlreadyCompleted):
		response.WriteError(w, r, http.StatusConflict, "already_verified", "Регистрация уже подтверждена.")
	case errors.Is(err, auth.ErrEmailAlreadyExists):
		response.WriteError(w, r, http.StatusConflict, "email_taken", "Пользователь с таким email уже зарегистрирован.")
	default:
		response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Не удалось подтвердить регистрацию.")
	}
}

func writeRegisterResendError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrVerificationNotFound):
		response.WriteError(w, r, http.StatusNotFound, "verification_not_found", "Регистрация с указанным кодом подтверждения не найдена.")
	case errors.Is(err, auth.ErrVerificationAlreadyCompleted):
		response.WriteError(w, r, http.StatusConflict, "already_verified", "Регистрация уже подтверждена.")
	case errors.Is(err, auth.ErrRegistrationResendTooEarly):
		response.WriteError(w, r, http.StatusTooManyRequests, "resend_too_early", "Код подтверждения пока нельзя отправить повторно.")
	case errors.Is(err, auth.ErrRegistrationResendLimitExceeded):
		response.WriteError(w, r, http.StatusTooManyRequests, "resend_limit_exceeded", "Превышен лимит повторной отправки кода подтверждения.")
	case errors.Is(err, auth.ErrVerificationCodeExpired):
		response.WriteError(w, r, http.StatusGone, "verification_expired", "Срок действия подтверждения регистрации истёк.")
	default:
		response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "Не удалось повторно отправить код подтверждения.")
	}
}
