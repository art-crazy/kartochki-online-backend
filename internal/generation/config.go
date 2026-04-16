package generation

import "context"

// CatalogOption описывает один вариант из каталога настроек генерации.
type CatalogOption struct {
	ID    string
	Label string
}

// CardTypeOption описывает один тип карточки для страницы генерации.
type CardTypeOption struct {
	ID              string
	Label           string
	DefaultSelected bool
}

// marketplaceOption хранит публичные поля marketplace и внутренние данные для генерации.
type marketplaceOption struct {
	ID          string
	Label       string
	AspectRatio string
	promptPart  string
}

// promptCatalogOption хранит публичные поля справочника и часть prompt для AI.
type promptCatalogOption struct {
	ID         string
	Label      string
	promptPart string
}

// promptCardTypeOption хранит публичные поля типа карточки и часть prompt для AI.
type promptCardTypeOption struct {
	ID              string
	Label           string
	DefaultSelected bool
	promptPart      string
}

// ModelOption описывает AI-модель, доступную пользователю при генерации.
// PricePerImage хранится в копейках, чтобы не использовать float для денег.
type ModelOption struct {
	ID            string
	Label         string
	Description   string
	PricePerImage int
}

// Config описывает справочные данные для страницы `/app/generate`.
type Config struct {
	Marketplaces     []CatalogOption
	Styles           []CatalogOption
	CardTypes        []CardTypeOption
	CardCountOptions []int
	Models           []ModelOption
}

var (
	// generateMaxCardCount задаёт верхнюю границу количества карточек в одном запуске.
	// Это значение должно совпадать с доменной валидацией и ответом /generate/config.
	generateMaxCardCount = 15
	// generateMarketplaces хранит marketplace вместе с aspect ratio и частью prompt.
	// Добавление нового marketplace должно менять только этот каталог и OpenAPI-ответ.
	generateMarketplaces = []marketplaceOption{
		{ID: "wildberries", Label: "Wildberries", AspectRatio: "3:4", promptPart: "Wildberries"},
		{ID: "ozon", Label: "Ozon", AspectRatio: "3:4", promptPart: "Ozon"},
		{ID: "yandex_market", Label: "Яндекс Маркет", AspectRatio: "1:1", promptPart: "Yandex Market"},
	}

	generateStyles = []promptCatalogOption{
		{ID: "clean_catalog", Label: "Чистый каталог", promptPart: "clean catalog, white background, minimal"},
		{ID: "accent_offer", Label: "Акцент на выгоде", promptPart: "accent on offer and benefits, vibrant, attention-grabbing"},
		{ID: "premium_brand", Label: "Премиальный бренд", promptPart: "premium brand, luxury, elegant, sophisticated"},
	}

	generateCardTypes = []promptCardTypeOption{
		{ID: "cover", Label: "Обложка", DefaultSelected: true, promptPart: "main product cover shot, hero image"},
		{ID: "benefits", Label: "Преимущества", DefaultSelected: true, promptPart: "product benefits and key features highlighted"},
		{ID: "details", Label: "Детали", DefaultSelected: true, promptPart: "product details and close-up view"},
		{ID: "usage", Label: "Сценарий использования", promptPart: "product in use, lifestyle scenario"},
		{ID: "dimensions", Label: "Размеры", promptPart: "product dimensions and measurements diagram"},
		{ID: "composition", Label: "Состав", promptPart: "product composition and materials"},
	}

	// generateModels хранит доступные AI-модели и цену одной картинки в копейках.
	// Первая модель считается выбором по умолчанию, если клиент не передал ModelID.
	generateModels = []ModelOption{
		{
			ID:            "google/gemini-2.5-flash-image",
			Label:         "Gemini 2.5 Flash",
			Description:   "Быстро и дёшево. Хорошо для прототипирования и большого количества карточек.",
			PricePerImage: 5,
		},
		{
			ID:            "google/gemini-3-pro-image-preview",
			Label:         "Gemini 3 Pro",
			Description:   "Высокое качество изображений, детальная проработка. Оптимален для финального результата.",
			PricePerImage: 25,
		},
		{
			ID:            "black-forest-labs/flux.2-pro",
			Label:         "FLUX.2 Pro",
			Description:   "Фотореалистичные изображения. Лучший выбор для товаров с реалистичной фотографией.",
			PricePerImage: 40,
		},
		{
			ID:            "openai/gpt-5-image-mini",
			Label:         "GPT-5 Image Mini",
			Description:   "Хорошее соотношение цены и качества от OpenAI.",
			PricePerImage: 15,
		},
	}
)

// GetConfig возвращает каталог вариантов для страницы `/app/generate`.
func (s *Service) GetConfig(_ context.Context) Config {
	return Config{
		Marketplaces:     marketplacesToCatalog(generateMarketplaces),
		Styles:           promptCatalogToCatalog(generateStyles),
		CardTypes:        promptCardTypesToCardTypes(generateCardTypes),
		CardCountOptions: buildCardCountOptions(generateMaxCardCount),
		Models:           cloneModelOptions(generateModels),
	}
}

func marketplacesToCatalog(items []marketplaceOption) []CatalogOption {
	result := make([]CatalogOption, len(items))
	for i, item := range items {
		result[i] = CatalogOption{ID: item.ID, Label: item.Label}
	}
	return result
}

func promptCatalogToCatalog(items []promptCatalogOption) []CatalogOption {
	result := make([]CatalogOption, len(items))
	for i, item := range items {
		result[i] = CatalogOption{ID: item.ID, Label: item.Label}
	}
	return result
}

func promptCardTypesToCardTypes(items []promptCardTypeOption) []CardTypeOption {
	result := make([]CardTypeOption, len(items))
	for i, item := range items {
		result[i] = CardTypeOption{
			ID:              item.ID,
			Label:           item.Label,
			DefaultSelected: item.DefaultSelected,
		}
	}
	return result
}

func cloneModelOptions(items []ModelOption) []ModelOption {
	return append([]ModelOption(nil), items...)
}

// buildCardCountOptions строит последовательность значений card_count для формы генерации.
func buildCardCountOptions(maxCount int) []int {
	if maxCount <= 0 {
		return nil
	}

	options := make([]int, 0, maxCount)
	for count := 1; count <= maxCount; count++ {
		options = append(options, count)
	}

	return options
}
