package billing

import (
	"context"
	"fmt"
)

// RemovePaymentMethod удаляет привязанную карту у пользователя и отключает автопродление.
// Если активной подписки с картой нет, возвращает ErrPaymentMethodNotFound.
func (s *Service) RemovePaymentMethod(ctx context.Context, userID string) error {
	uid, err := parseUserID(userID)
	if err != nil {
		return ErrUserNotFound
	}

	affected, err := s.queries.ClearSubscriptionPaymentMethod(ctx, uid)
	if err != nil {
		return fmt.Errorf("clear subscription payment method: %w", err)
	}
	if affected == 0 {
		return ErrPaymentMethodNotFound
	}

	return nil
}
