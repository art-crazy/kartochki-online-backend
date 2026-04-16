package generation

import "context"

// ImageGenerateInput описывает параметры одной карточки, которую нужно сгенерировать.
type ImageGenerateInput struct {
	// Prompt — текстовое описание желаемого изображения.
	Prompt string
	// SourceImageBody содержит байты исходного изображения пользователя.
	// Если поле пустое, провайдер получает обычный text-to-image запрос.
	SourceImageBody []byte
	// SourceImageMIMEType хранит MIME-тип исходника для сборки data URL в multimodal-запросе.
	SourceImageMIMEType string
	// AspectRatio — соотношение сторон в формате "W:H", например "3:4".
	AspectRatio string
	// ModelID — идентификатор модели в RouterAI, например "google/gemini-2.5-flash-image".
	ModelID string
}

// ImageGenerator — контракт для любого провайдера генерации изображений.
// Реализует internal/platform/routerai.Client; при отсутствии ключа используется noopImageGenerator.
type ImageGenerator interface {
	GenerateImage(ctx context.Context, input ImageGenerateInput) ([]byte, error)
}

// UploadedImage описывает исходное изображение, которое frontend прислал через upload endpoint.
type UploadedImage struct {
	FileName    string
	ContentType string
	Body        []byte
}

// UploadedAsset описывает результат сохранения исходного изображения.
type UploadedAsset struct {
	AssetID    string
	PreviewURL string
}

// CreateInput описывает запуск новой генерации карточек.
type CreateInput struct {
	UserID        string
	ProjectName   string
	MarketplaceID string
	StyleID       string
	CardTypeIDs   []string
	CardCount     int
	SourceAssetID string
	// ModelID — идентификатор AI-модели из каталога generateModels.
	// Если пустой, используется первая модель из каталога.
	ModelID string
}

// CreatedGeneration описывает результат постановки генерации в очередь.
type CreatedGeneration struct {
	GenerationID string
	Status       string
}

// GeneratedCard описывает одну готовую карточку в ответе polling endpoint.
type GeneratedCard struct {
	ID         string
	CardTypeID string
	AssetID    string
	PreviewURL string
}

// Status описывает текущее состояние генерации и уже доступные артефакты.
type Status struct {
	GenerationID       string
	Status             string
	CurrentStep        string
	ProgressPercent    int
	ErrorMessage       string
	ArchiveDownloadURL string
	ResultCards        []GeneratedCard
}
