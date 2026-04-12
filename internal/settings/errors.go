package settings

import "errors"

var (
	// ErrUserNotFound возвращается, когда пользователь уже удалён или id некорректен.
	ErrUserNotFound = errors.New("settings user not found")
	// ErrEmailTaken возвращается, когда новый email уже занят другим пользователем.
	ErrEmailTaken = errors.New("settings email already taken")
	// ErrNameRequired возвращается, когда профиль пытаются сохранить без имени.
	ErrNameRequired = errors.New("settings name is required")
	// ErrCardsPerGenerationOutOfRange возвращается при недопустимом количестве карточек по умолчанию.
	ErrCardsPerGenerationOutOfRange = errors.New("settings cards per generation is out of range")
	// ErrInvalidImageFormat возвращается, когда frontend прислал неизвестный формат изображения.
	ErrInvalidImageFormat = errors.New("settings image format is invalid")
	// ErrUnknownNotificationKey возвращается, когда frontend пытается сохранить неизвестный ключ уведомления.
	ErrUnknownNotificationKey = errors.New("settings notification key is unknown")
	// ErrCurrentPasswordInvalid возвращается, когда текущий пароль не совпадает.
	ErrCurrentPasswordInvalid = errors.New("settings current password is invalid")
	// ErrSessionNotFound возвращается, когда указанная пользовательская сессия не найдена.
	ErrSessionNotFound = errors.New("settings session not found")
	// ErrCannotRevokeCurrentSession возвращается, когда пользователь пытается отозвать текущую сессию.
	ErrCannotRevokeCurrentSession = errors.New("settings current session cannot be revoked")
	// ErrInvalidConfirmWord возвращается, когда удаление аккаунта подтверждено неверным словом.
	ErrInvalidConfirmWord = errors.New("settings confirm word is invalid")
)
