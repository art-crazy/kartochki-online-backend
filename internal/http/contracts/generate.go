package contracts

// GenerateConfigResponse описывает справочные данные для страницы генерации.
type GenerateConfigResponse struct {
	Marketplaces     []GenerateMarketplace `json:"marketplaces"`
	Styles           []GenerateStyle       `json:"styles"`
	CardTypes        []GenerateCardType    `json:"card_types"`
	CardCountOptions []int                 `json:"card_count_options"`
}

// GenerateMarketplace описывает одну доступную площадку генерации.
type GenerateMarketplace struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// GenerateStyle описывает один доступный стиль карточек.
type GenerateStyle struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// GenerateCardType описывает один доступный тип карточки.
type GenerateCardType struct {
	ID              string `json:"id"`
	Label           string `json:"label"`
	DefaultSelected bool   `json:"default_selected,omitempty"`
}

// UploadImageResponse возвращается после загрузки исходного изображения.
type UploadImageResponse struct {
	AssetID    string `json:"asset_id"`
	PreviewURL string `json:"preview_url"`
}

// CreateGenerationRequest описывает запуск генерации карточек.
type CreateGenerationRequest struct {
	ProjectName   string   `json:"project_name,omitempty"`
	MarketplaceID string   `json:"marketplace_id"`
	StyleID       string   `json:"style_id"`
	CardTypeIDs   []string `json:"card_type_ids"`
	CardCount     int      `json:"card_count"`
	SourceAssetID string   `json:"source_asset_id"`
}

// CreateGenerationResponse возвращается сразу после постановки генерации в работу.
type CreateGenerationResponse struct {
	GenerationID string `json:"generation_id"`
	Status       string `json:"status"`
}

// GenerationStatusResponse описывает текущее состояние генерации и результаты.
type GenerationStatusResponse struct {
	GenerationID       string          `json:"generation_id"`
	Status             string          `json:"status"`
	CurrentStep        string          `json:"current_step,omitempty"`
	ProgressPercent    int             `json:"progress_percent,omitempty"`
	ErrorMessage       string          `json:"error_message,omitempty"`
	ArchiveDownloadURL string          `json:"archive_download_url,omitempty"`
	ResultCards        []GeneratedCard `json:"result_cards,omitempty"`
}

// GeneratedCard описывает одну сгенерированную карточку товара.
type GeneratedCard struct {
	ID         string `json:"id"`
	CardTypeID string `json:"card_type_id"`
	AssetID    string `json:"asset_id"`
	PreviewURL string `json:"preview_url"`
}
