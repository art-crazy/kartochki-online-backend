package generation

import "errors"

var (
	// ErrSourceAssetNotFound означает, что исходное изображение не найдено или не принадлежит пользователю.
	ErrSourceAssetNotFound = errors.New("generation source asset not found")
	// ErrGenerationNotFound означает, что генерация не найдена или не принадлежит пользователю.
	ErrGenerationNotFound = errors.New("generation not found")
	// ErrInvalidMarketplace означает, что marketplace_id не входит в поддерживаемый каталог.
	ErrInvalidMarketplace = errors.New("generation marketplace is invalid")
	// ErrInvalidStyle означает, что style_id не входит в поддерживаемый каталог.
	ErrInvalidStyle = errors.New("generation style is invalid")
	// ErrInvalidCardType означает, что хотя бы один card_type_id не входит в поддерживаемый каталог.
	ErrInvalidCardType = errors.New("generation card type is invalid")
	// ErrInvalidCardCount означает, что card_count не входит в разрешённый список вариантов.
	ErrInvalidCardCount = errors.New("generation card count is invalid")
	// ErrProjectNameTooLong означает, что project_name не помещается в ограничение projects.
	ErrProjectNameTooLong = errors.New("generation project name is too long")
	// ErrImageRequired означает, что upload endpoint получил пустой файл.
	ErrImageRequired = errors.New("generation image is required")
	// ErrImageTypeNotSupported означает, что backend не умеет безопасно принять этот тип файла.
	ErrImageTypeNotSupported = errors.New("generation image type is not supported")
	// ErrQuotaExceeded означает, что новый запуск генерации не помещается в текущий billing-лимит.
	ErrQuotaExceeded = errors.New("generation quota exceeded")
)
