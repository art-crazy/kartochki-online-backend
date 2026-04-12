package projects

import "errors"

// ErrNotFound возвращается, когда проект не найден, уже удалён или не принадлежит пользователю.
var ErrNotFound = errors.New("project not found")

// ErrTitleRequired возвращается, когда проект пытаются создать или обновить без названия.
var ErrTitleRequired = errors.New("project title is required")

// ErrTitleTooLong возвращается, когда название проекта слишком длинное для стабильной работы UI и логов.
var ErrTitleTooLong = errors.New("project title is too long")

// ErrMarketplaceTooLong возвращается, когда идентификатор маркетплейса превышает допустимую длину.
var ErrMarketplaceTooLong = errors.New("project marketplace is too long")

// ErrProductNameTooLong возвращается, когда название товара не помещается в разумный лимит проекта.
var ErrProductNameTooLong = errors.New("project product name is too long")

// ErrProductDescriptionTooLong возвращается, когда описание товара слишком большое для одного проекта.
var ErrProductDescriptionTooLong = errors.New("project product description is too long")
