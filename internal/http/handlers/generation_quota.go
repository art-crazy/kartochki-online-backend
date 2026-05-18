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
		"Лимит генераций на текущий период исчерпан. Увеличьте лимит в тарифе или попробуйте снова после обновления периода.",
	)
}

func buildGenerationQuotaMessage(err generation.QuotaExceededError) string {
	availableCards := max(err.CardsLimit-err.ReservedCards, 0)
	base := fmt.Sprintf(
		"Для этого запуска не хватает карточек: доступно %s, нужно %s.",
		russianCards(availableCards),
		russianCards(err.RequestedCards),
	)
	if availableCards == 0 {
		base = fmt.Sprintf(
			"Лимит карточек на текущий период исчерпан: доступно %s, нужно %s.",
			russianCards(availableCards),
			russianCards(err.RequestedCards),
		)
	}

	if !err.PeriodEnd.IsZero() {
		return fmt.Sprintf("%s Лимит обновится %s. Также можно увеличить лимит в разделе тарифов.", base, formatUserDate(err.PeriodEnd))
	}

	return base + " Увеличьте лимит в разделе тарифов или попробуйте снова после обновления периода."
}

func buildGenerationQuotaDetails(err generation.QuotaExceededError) []openapi.ErrorDetail {
	availableCards := max(err.CardsLimit-err.ReservedCards, 0)
	details := []openapi.ErrorDetail{
		{
			Field:   strPtr("cards_limit"),
			Message: fmt.Sprintf("Лимит тарифа: %s на текущий период.", russianCards(err.CardsLimit)),
		},
		{
			Field: strPtr("reserved_cards"),
			Message: fmt.Sprintf(
				"Уже занято: %s, включая готовые карточки и генерации в очереди или обработке.",
				russianCards(err.ReservedCards),
			),
		},
		{
			Field:   strPtr("requested_cards"),
			Message: fmt.Sprintf("Для этого запуска нужно: %s.", russianCards(err.RequestedCards)),
		},
		{
			Field:   strPtr("available_cards"),
			Message: fmt.Sprintf("Доступно сейчас: %s.", russianCards(availableCards)),
		},
	}

	if !err.PeriodEnd.IsZero() {
		details = append(details, openapi.ErrorDetail{
			Field:   strPtr("period_end"),
			Message: fmt.Sprintf("Лимит обновится %s.", formatUserDate(err.PeriodEnd)),
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
