package projects

import "errors"

// ErrNotFound возвращается, когда проект не найден или не принадлежит пользователю.
var ErrNotFound = errors.New("project not found")
