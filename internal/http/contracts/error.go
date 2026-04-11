package contracts

// ErrorResponse описывает единый формат ошибки для публичного HTTP API.
type ErrorResponse struct {
	Code      string        `json:"code"`
	Message   string        `json:"message"`
	RequestID string        `json:"request_id,omitempty"`
	Details   []ErrorDetail `json:"details,omitempty"`
}

// ErrorDetail описывает дополнительную деталь ошибки, например проблемное поле.
type ErrorDetail struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}
