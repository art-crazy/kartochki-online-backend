package generation

// styleRule возвращает блок правил визуального стиля для prompt.
func styleRule(styleID string) string {
	switch styleID {
	case "clean_catalog":
		return "Visual style: clean catalog design, light background, minimal layout, lots of whitespace, calm colors.\n" +
			"Use text only where it helps explain the product."
	case "accent_offer":
		return "Visual style: vibrant offer-focused marketplace design, bright accent blocks, dynamic composition, high contrast, attention-grabbing but not chaotic.\n" +
			"Use 2-4 short text blocks when product context is available."
	case "premium_brand":
		return "Visual style: premium brand look, elegant composition, restrained colors, expensive feel, careful typography, minimal but strong visual hierarchy.\n" +
			"Use a refined editorial composition where the product remains the hero object.\n" +
			"Prefer one concise headline and very limited text.\n" +
			"Avoid cheap discount-like graphics, crowded bullet lists and generic template layouts."
	default:
		return "Visual style: clean and professional marketplace design, clear hierarchy, readable text."
	}
}
