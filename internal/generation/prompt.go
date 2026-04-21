package generation

import "strings"

// qualityBlock — общий блок требований к качеству и безопасности, добавляется в каждый prompt.
// Он запрещает выдумывать свойства товара, выпускать текст с ошибками и уводить фокус с товара.
const qualityBlock = `Important quality rules:
- Preserve the original product shape, color, proportions and visible logo as accurately as possible.
- Do not redesign the product itself.
- Keep the product as the main visual focus. A model or lifestyle scene may support the composition, but must not dominate over the product.
- Do not invent brand claims, certificates, discounts, ratings or exact numbers.
- Use only facts that are explicitly provided in the input product context.
- If some benefit, material, feature or measurement is not provided, do not mention it in text.
- Do not add fake marketplace badges, fake reviews or fake official labels.
- All text must be in Russian, short and readable.
- Use correct Russian spelling and grammar.
- Proofread every Russian word before returning the final image.
- Text must not cover the main product.
- Avoid generic filler bullets and avoid awkward or unnatural product wording.
- Do not return only a cut-out product photo or a plain background replacement.
- Build a complete marketplace card layout with composition, visual hierarchy, readable Russian text and product-focused design.
- The result must look like a finished e-commerce product card.`

// BuildPrompt собирает prompt из четырёх блоков: задача карточки, marketplace, стиль, quality.
// Описание блоков оставлено на английском, потому что image-модели лучше реагируют на английские prompt.
// Если Product nil, шаблоны строятся без конкретных деталей товара — модель не выдумывает бренды и числа.
func BuildPrompt(input PromptInput) string {
	marketplace := marketplaceDisplayName(input.MarketplaceID)

	parts := []string{
		cardTypeBlock(input.CardTypeID, marketplace, input.Product),
		marketplaceRule(input.MarketplaceID),
		styleRule(input.StyleID),
		qualityBlock,
	}

	return strings.Join(parts, "\n\n")
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
