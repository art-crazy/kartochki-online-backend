package handlers

import (
	"strings"
	"testing"
	"time"

	"kartochki-online-backend/internal/generation"
)

func TestBuildGenerationQuotaMessageIncludesUserDetails(t *testing.T) {
	message := buildGenerationQuotaMessage(generation.QuotaExceededError{
		CardsLimit:     5,
		ReservedCards:  5,
		RequestedCards: 1,
		PeriodEnd:      time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})

	expectedParts := []string{
		"Недостаточно карточек в текущем лимите",
		"Доступно: 0",
		"Нужно: 1",
		"Обновление лимита: 01.06.2026",
	}
	for _, part := range expectedParts {
		if !strings.Contains(message, part) {
			t.Fatalf("message %q does not contain %q", message, part)
		}
	}
}

func TestBuildGenerationQuotaDetailsUsesRussianPluralForms(t *testing.T) {
	details := buildGenerationQuotaDetails(generation.QuotaExceededError{
		CardsLimit:     21,
		ReservedCards:  19,
		RequestedCards: 3,
		PeriodEnd:      time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})

	messages := make([]string, 0, len(details))
	for _, detail := range details {
		messages = append(messages, detail.Message)
	}
	joinedMessages := strings.Join(messages, "\n")

	expectedParts := []string{
		"21 карточка",
		"19 карточек",
		"3 карточки",
		"2 карточки",
	}
	for _, part := range expectedParts {
		if !strings.Contains(joinedMessages, part) {
			t.Fatalf("details %q do not contain %q", joinedMessages, part)
		}
	}
}
