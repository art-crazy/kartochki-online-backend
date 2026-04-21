package generation

// marketplaceRule возвращает блок правил marketplace для prompt.
// Описание оставлено на английском, потому что image-модели лучше понимают английские prompt.
func marketplaceRule(marketplaceID string) string {
	switch marketplaceID {
	case "wildberries":
		return "Marketplace: Wildberries.\n" +
			"Use vertical 3:4 composition.\n" +
			"Create a finished Russian marketplace product card.\n" +
			"Use clear hierarchy: product, headline, benefits/callouts.\n" +
			"Text must be in Russian and readable on mobile."
	case "ozon":
		return "Marketplace: Ozon.\n" +
			"Use vertical 3:4 composition.\n" +
			"Create a clean e-commerce card with strong product focus.\n" +
			"Use readable Russian text, benefit blocks and neat spacing."
	case "yandex_market":
		return "Marketplace: Yandex Market.\n" +
			"Use square 1:1 composition.\n" +
			"Create a clean product-focused e-commerce card.\n" +
			"Keep the design clear, not overloaded.\n" +
			"Use readable Russian text."
	default:
		return "Marketplace: general e-commerce.\n" +
			"Use vertical 3:4 composition.\n" +
			"Create a clean product card with clear hierarchy and Russian text."
	}
}
