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

// marketplaceOption хранит поля marketplace, включая aspect ratio для prompt builder.
type marketplaceOption struct {
	ID          string
	Label       string
	AspectRatio string
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

	// generateMarketplaces хранит marketplace вместе с aspect ratio для prompt builder.
	// Добавление нового marketplace должно менять этот каталог, marketplaceRule и OpenAPI-ответ.
	generateMarketplaces = []marketplaceOption{
		{ID: "wildberries", Label: "Wildberries", AspectRatio: "3:4"},
		{ID: "ozon", Label: "Ozon", AspectRatio: "3:4"},
		{ID: "yandex_market", Label: "Яндекс Маркет", AspectRatio: "1:1"},
	}

	generateStyles = []CatalogOption{
		{ID: "clean_catalog", Label: "Чистый каталог"},
		{ID: "accent_offer", Label: "Акцент на выгоде"},
		{ID: "premium_brand", Label: "Премиальный бренд"},
	}

	generateCardTypes = []CardTypeOption{
		{ID: "cover", Label: "Обложка", DefaultSelected: true},
		{ID: "benefits", Label: "Преимущества", DefaultSelected: true},
		{ID: "details", Label: "Детали", DefaultSelected: true},
		{ID: "usage", Label: "Сценарий использования"},
		{ID: "dimensions", Label: "Размеры"},
		{ID: "composition", Label: "Состав"},
	}

	// generateModels хранит доступные AI-модели и цену одной картинки в копейках.
	// Первая модель считается выбором по умолчанию, если клиент не передал ModelID.
	generateModels = []ModelOption{
		{
			ID:            "google/gemini-3.1-flash-image-preview",
			Label:         "Gemini 3.1 Flash Image Preview",
			Description:   "Быстрая модель Google для генерации и редактирования изображений с хорошим качеством.",
			PricePerImage: 10,
		},
		{
			ID:            "openai/gpt-5-image",
			Label:         "GPT-5 Image",
			Description:   "Флагманская мультимодальная модель OpenAI для генерации изображений и сложных инструкций.",
			PricePerImage: 50,
		},
		{
			ID:            "black-forest-labs/flux.2-flex",
			Label:         "FLUX.2 Flex",
			Description:   "Производительная модель FLUX.2 с балансом качества и скорости для production-сценариев.",
			PricePerImage: 35,
		},
		{
			ID:            "black-forest-labs/flux.2-max",
			Label:         "FLUX.2 Max",
			Description:   "Сильная модель FLUX.2 для сложного текста, типографики и мелких деталей.",
			PricePerImage: 45,
		},
		{
			ID:            "black-forest-labs/flux.2-klein-4b",
			Label:         "FLUX.2 Klein 4B",
			Description:   "Самая быстрая и экономичная модель в линейке FLUX.2 для массовых генераций.",
			PricePerImage: 10,
		},
		{
			ID:            "bytedance-seed/seedream-4.5",
			Label:         "Seedream 4.5",
			Description:   "Новая модель ByteDance для генерации изображений с улучшенной согласованностью редактирования и детализацией.",
			PricePerImage: 20,
		},
		{
			ID:            "openai/gpt-5-image-mini",
			Label:         "GPT-5 Image Mini",
			Description:   "Хорошее соотношение цены и качества от OpenAI.",
			PricePerImage: 15,
		},
		{
			ID:            "black-forest-labs/flux.2-pro",
			Label:         "FLUX.2 Pro",
			Description:   "Фотореалистичные изображения. Лучший выбор для товаров с реалистичной фотографией.",
			PricePerImage: 40,
		},
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
			ID:            "openai/gpt-5.4-image-2",
			Label:         "GPT-5.4 Image 2",
			Description:   "Новая мультимодальная модель OpenAI для генерации изображений с сильным качеством и пониманием сложных инструкций.",
			PricePerImage: 30,
		},
	}
)

// GetConfig возвращает каталог вариантов для страницы `/app/generate`.
func (s *Service) GetConfig(_ context.Context) Config {
	return Config{
		Marketplaces:     marketplacesToCatalog(generateMarketplaces),
		Styles:           append([]CatalogOption(nil), generateStyles...),
		CardTypes:        append([]CardTypeOption(nil), generateCardTypes...),
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
