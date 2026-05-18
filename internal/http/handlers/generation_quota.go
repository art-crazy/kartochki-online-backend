package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	openapi "kartochki-online-backend/api/gen"
	"kartochki-online-backend/internal/generation"
	"kartochki-online-backend/internal/http/response"
)

// writeGenerationQuotaError отдаёт понятную ошибку, когда запуск не помещается в лимит тарифа.
func writeGenerationQuotaError(w http.ResponseWriter, r *http.Request, err error) {
	var quotaErr generation.QuotaExceededError
	if errors.As(err, &quotaErr) {
		response.WriteError(
			w,
			r,
			http.StatusConflict,
			"generation_quota_exceeded",
			buildGenerationQuotaMessage(quotaErr),
			buildGenerationQuotaDetails(quotaErr)...,
		)
		return
	}

	response.WriteError(
		w,
		r,
		http.StatusConflict,
		"generation_quota_exceeded",
		"Недостаточно карточек в текущем лимите. Можно сменить тариф или дождаться обновления лимита.",
	)
}

func buildGenerationQuotaMessage(err generation.QuotaExceededError) string {
	availableCards := max(err.CardsLimit-err.ReservedCards, 0)
	message := fmt.Sprintf(
		"Недостаточно карточек в текущем лимите. Доступно: %d. Нужно: %d.",
		availableCards,
		err.RequestedCards,
	)
	if availableCards > 0 {
		message = fmt.Sprintf(
			"Не хватает карточек для запуска. Доступно: %d. Нужно: %d.",
			availableCards,
			err.RequestedCards,
		)
	}

	if !err.PeriodEnd.IsZero() {
		return fmt.Sprintf("%s Обновление лимита: %s.", message, formatUserDate(err.PeriodEnd))
	}

	return message + " Можно сменить тариф или дождаться обновления лимита."
}

func buildGenerationQuotaDetails(err generation.QuotaExceededError) []openapi.ErrorDetail {
	availableCards := max(err.CardsLimit-err.ReservedCards, 0)
	details := []openapi.ErrorDetail{
		{
			Field:   strPtr("cards_limit"),
			Message: fmt.Sprintf("Лимит тарифа: %s.", russianCards(err.CardsLimit)),
		},
		{
			Field: strPtr("reserved_cards"),
			Message: fmt.Sprintf(
				"Уже занято: %s. Сюда входят готовые карточки и запуски в очереди.",
				russianCards(err.ReservedCards),
			),
		},
		{
			Field:   strPtr("requested_cards"),
			Message: fmt.Sprintf("Нужно для запуска: %s.", russianCards(err.RequestedCards)),
		},
		{
			Field:   strPtr("available_cards"),
			Message: fmt.Sprintf("Осталось: %s.", russianCards(availableCards)),
		},
	}

	if !err.PeriodEnd.IsZero() {
		details = append(details, openapi.ErrorDetail{
			Field:   strPtr("period_end"),
			Message: fmt.Sprintf("Обновление лимита: %s.", formatUserDate(err.PeriodEnd)),
		})
	}

	return details
}

func formatUserDate(value time.Time) string {
	return value.In(time.Local).Format("02.01.2006")
}

func russianCards(count int) string {
	absCount := count
	if absCount < 0 {
		absCount = -absCount
	}

	lastTwoDigits := absCount % 100
	if lastTwoDigits >= 11 && lastTwoDigits <= 14 {
		return fmt.Sprintf("%d карточек", count)
	}

	switch absCount % 10 {
	case 1:
		return fmt.Sprintf("%d карточка", count)
	case 2, 3, 4:
		return fmt.Sprintf("%d карточки", count)
	default:
		return fmt.Sprintf("%d карточек", count)
	}
}
