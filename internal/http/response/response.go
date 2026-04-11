// Package response централизует JSON-ответы HTTP-слоя.
package response

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"kartochki-online-backend/internal/http/contracts"
)

const requestIDHeader = "X-Request-ID"

// WriteJSON пишет JSON-ответ и пробрасывает request ID в заголовок ответа.
func WriteJSON(w http.ResponseWriter, r *http.Request, status int, payload any) {
	if r != nil {
		if requestID := middleware.GetReqID(r.Context()); requestID != "" {
			w.Header().Set(requestIDHeader, requestID)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// WriteError пишет ошибку в едином формате, чтобы фронтенд не разбирал разные схемы.
func WriteError(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	code string,
	message string,
	details ...contracts.ErrorDetail,
) {
	payload := contracts.ErrorResponse{
		Code:    code,
		Message: message,
		Details: details,
	}

	if r != nil {
		payload.RequestID = middleware.GetReqID(r.Context())
	}

	WriteJSON(w, r, status, payload)
}
