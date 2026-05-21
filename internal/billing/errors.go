package billing

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrUserNotFound возвращается, когда billing-сценарий запрошен для несуществующего пользователя.
	ErrUserNotFound = errors.New("billing user not found")
	// ErrFreePlanNotFound возвращается, когда таблица plans не содержит строку с кодом "free".
	// Это означает, что нужная миграция не была применена к базе данных.
	ErrFreePlanNotFound = errors.New("billing free plan not found in database")
	// ErrPlanNotFound возвращается, когда frontend передал неизвестный план.
	ErrPlanNotFound = errors.New("billing plan not found")
	// ErrAddonNotFound возвращается, когда frontend передал неизвестный addon-пакет.
	ErrAddonNotFound = errors.New("billing addon not found")
	// ErrInvalidPlanPeriod возвращается, когда checkout запрашивает неподдерживаемый период оплаты.
	ErrInvalidPlanPeriod = errors.New("billing plan period is invalid")
	// ErrPlanAlreadyActive возвращается, когда пользователь пытается купить уже активный тариф.
	ErrPlanAlreadyActive = errors.New("billing plan is already active")
	// ErrCheckoutProviderNotConfigured возвращается, когда backend ещё не подключён к платёжному провайдеру.
	ErrCheckoutProviderNotConfigured = errors.New("billing checkout provider is not configured")
	// ErrCheckoutProviderFailed возвращается, когда платёжный провайдер отклонил создание checkout.
	ErrCheckoutProviderFailed = errors.New("billing checkout provider failed")
	// ErrCheckoutPersistenceFailed возвращается, когда checkout создан у провайдера, но не сохранён в БД.
	ErrCheckoutPersistenceFailed = errors.New("billing checkout persistence failed")
	// ErrCheckoutPreparationFailed возвращается, когда backend не смог подготовить данные для checkout.
	ErrCheckoutPreparationFailed = errors.New("billing checkout preparation failed")
	// ErrCheckoutEmailRequired возвращается, когда для создания чека не хватает email пользователя.
	ErrCheckoutEmailRequired = errors.New("billing checkout email is required")
	// ErrCheckoutEmailInvalid возвращается, когда email для чека имеет некорректный формат.
	ErrCheckoutEmailInvalid = errors.New("billing checkout email is invalid")
	// ErrCheckoutEmailTaken возвращается, когда email для чека уже принадлежит другому пользователю.
	ErrCheckoutEmailTaken = errors.New("billing checkout email is already taken")
	// ErrSubscriptionNotCancelable возвращается, когда у пользователя нет платной подписки для отмены.
	ErrSubscriptionNotCancelable = errors.New("billing subscription is not cancelable")
	// ErrGenerationLimitExceeded возвращается, когда новый запуск генерации превысит доступный лимит карточек.
	ErrGenerationLimitExceeded = errors.New("billing generation limit exceeded")
	// ErrPaymentMethodNotFound возвращается, когда у пользователя нет привязанной карты для удаления.
	ErrPaymentMethodNotFound = errors.New("billing payment method not found")
)

// GenerationLimitExceededError описывает, почему генерация не прошла проверку лимита.
// HTTP-слой использует эти числа, чтобы показать пользователю понятное сообщение.
type GenerationLimitExceededError struct {
	CardsLimit     int
	ReservedCards  int
	RequestedCards int
	PeriodEnd      time.Time
}

// Error возвращает короткий технический текст для логов и errors.Is.
func (e GenerationLimitExceededError) Error() string {
	return fmt.Sprintf(
		"%v: limit=%d reserved=%d requested=%d",
		ErrGenerationLimitExceeded,
		e.CardsLimit,
		e.ReservedCards,
		e.RequestedCards,
	)
}

// Is позволяет errors.Is распознавать ошибку как ErrGenerationLimitExceeded.
func (e GenerationLimitExceededError) Is(target error) bool {
	return target == ErrGenerationLimitExceeded
}
