package health

import (
	"context"
	"errors"
)

type Checker interface {
	Name() string
	Check(context.Context) error
}

type Service struct {
	checkers []Checker
}

func NewService(checkers ...Checker) *Service {
	return &Service{checkers: checkers}
}

func (s *Service) Ready(ctx context.Context) error {
	var joined error

	for _, checker := range s.checkers {
		if err := checker.Check(ctx); err != nil {
			joined = errors.Join(joined, errors.New(checker.Name()+": "+err.Error()))
		}
	}

	return joined
}

type CheckerFunc struct {
	name string
	fn   func(context.Context) error
}

func NewChecker(name string, fn func(context.Context) error) CheckerFunc {
	return CheckerFunc{name: name, fn: fn}
}

func (c CheckerFunc) Name() string {
	return c.name
}

func (c CheckerFunc) Check(ctx context.Context) error {
	return c.fn(ctx)
}

