package blog

import (
	"errors"
	"fmt"
)

var (
	// ErrPostNotFound означает, что опубликованная статья по slug не найдена.
	ErrPostNotFound = errors.New("blog post not found")
	// ErrInvalidPage означает, что параметр page не прошёл валидацию.
	ErrInvalidPage = errors.New("page must be greater than zero")
	// ErrInvalidPageSize означает, что параметр page_size выходит за допустимые границы.
	// Значение верхней границы берётся из MaxPageSize, чтобы текст ошибки не расходился с константой.
	ErrInvalidPageSize = fmt.Errorf("page_size must be between 1 and %d", MaxPageSize)
)
