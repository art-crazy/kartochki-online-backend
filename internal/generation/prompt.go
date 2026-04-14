package generation

import "fmt"

// buildCardPrompt составляет prompt для генерации одной карточки marketplace.
// Описание оставлено на английском, потому что image-модели обычно лучше понимают английские prompt.
func buildCardPrompt(marketplaceID, styleID, cardTypeID string) string {
	marketplace := findMarketplacePromptPart(generateMarketplaces, marketplaceID, "marketplace")
	style := findCatalogPromptPart(generateStyles, styleID, "clean and professional")
	cardType := findCardTypePromptPart(generateCardTypes, cardTypeID, "product image")

	return fmt.Sprintf(
		"Create a professional product card image for %s marketplace. Style: %s. Card type: %s. "+
			"High quality, clean background, suitable for e-commerce listing. No text overlays.",
		marketplace, style, cardType,
	)
}

// marketplaceAspectRatio возвращает рекомендуемое соотношение сторон карточки для marketplace.
func marketplaceAspectRatio(marketplaceID string) string {
	for _, m := range generateMarketplaces {
		if m.ID == marketplaceID {
			return m.AspectRatio
		}
	}
	return "3:4"
}

func findMarketplacePromptPart(items []marketplaceOption, id, fallback string) string {
	for _, item := range items {
		if item.ID == id {
			return item.promptPart
		}
	}
	return fallback
}

func findCatalogPromptPart(items []promptCatalogOption, id, fallback string) string {
	for _, item := range items {
		if item.ID == id {
			return item.promptPart
		}
	}
	return fallback
}

func findCardTypePromptPart(items []promptCardTypeOption, id, fallback string) string {
	for _, item := range items {
		if item.ID == id {
			return item.promptPart
		}
	}
	return fallback
}
