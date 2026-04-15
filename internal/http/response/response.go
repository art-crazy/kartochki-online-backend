// Package response централизует JSON-ответы HTTP-слоя.
package response

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	openapi "kartochki-online-backend/api/gen"
)

const requestIDHeader = "X-Request-ID"

// WriteJSON пишет JSON-ответ и пробрасывает request ID в заголовок ответа.
func WriteJSON(w http.ResponseWriter, r *http.Request, status int, payload any) {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":"internal_error","message":"failed to encode response"}` + "\n"))
		return
	}

	if r != nil {
		if requestID := middleware.GetReqID(r.Context()); requestID != "" {
			w.Header().Set(requestIDHeader, requestID)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}

// WriteError пишет ошибку в едином формате, чтобы фронтенд не разбирал разные схемы.
func WriteError(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	code string,
	message string,
	details ...openapi.ErrorDetail,
) {
	payload := openapi.ErrorResponse{
		Code:    code,
		Message: message,
	}

	if len(details) > 0 {
		payload.Details = &details
	}

	if r != nil {
		if reqID := middleware.GetReqID(r.Context()); reqID != "" {
			payload.RequestId = &reqID
		}
	}

	WriteJSON(w, r, status, payload)
}
