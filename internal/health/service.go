package health

import (
	"context"
	"errors"
	"fmt"
)

// Checker описывает одну проверку готовности подсистемы.
type Checker interface {
	Name() string
	Check(context.Context) error
}

// Service последовательно запускает проверки готовности приложения.
type Service struct {
	checkers []Checker
}

// NewService создаёт сервис readiness-проверок.
func NewService(checkers ...Checker) *Service {
	return &Service{checkers: checkers}
}

// Ready запускает все проверки и объединяет найденные ошибки.
func (s *Service) Ready(ctx context.Context) error {
	var joined error

	for _, checker := range s.checkers {
		if err := checker.Check(ctx); err != nil {
			joined = errors.Join(joined, fmt.Errorf("%s: %w", checker.Name(), err))
		}
	}

	return joined
}

// CheckerFunc адаптирует обычную функцию к интерфейсу Checker.
type CheckerFunc struct {
	name string
	fn   func(context.Context) error
}

// NewChecker создаёт именованную readiness-проверку.
func NewChecker(name string, fn func(context.Context) error) CheckerFunc {
	return CheckerFunc{name: name, fn: fn}
}

// Name возвращает имя проверки для логов и ошибок.
func (c CheckerFunc) Name() string {
	return c.name
}

// Check запускает саму проверку.
func (c CheckerFunc) Check(ctx context.Context) error {
	return c.fn(ctx)
}
