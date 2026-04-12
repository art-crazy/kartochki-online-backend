package projects

import "errors"

// ErrNotFound возвращается, когда проект не найден или не принадлежит пользователю.
var ErrNotFound = errors.New("project not found")

// ErrTitleRequired возвращается, когда проект пытаются создать или обновить без названия.
var ErrTitleRequired = errors.New("project title is required")
