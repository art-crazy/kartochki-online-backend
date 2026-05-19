package generation

import (
	"fmt"
	"strings"
)

// cardTypeBlock возвращает блок задачи карточки для указанного card_type_id.
// marketplaceName — человекочитаемое название для подстановки в текст.
// Если product nil, используется вариант без конкретных деталей товара.
func cardTypeBlock(cardTypeID, marketplaceName string, product *ProductContext) string {
	switch cardTypeID {
	// Текущие ID каталога.
	case "main":
		return coverBlock(marketplaceName, product)
	case "infographic", "advantages":
		return benefitsBlock(marketplaceName, product)
	case "details":
		return detailsBlock(marketplaceName, product)
	case "sizes":
		return dimensionsBlock(marketplaceName, product)
	case "composition":
		return compositionBlock(marketplaceName, product)
	case "use-case":
		return useCaseBlock(marketplaceName, product)
	case "packaging":
		return packagingBlock(marketplaceName, product)
	case "comparison":
		return comparisonBlock(marketplaceName, product)
	default:
		return fmt.Sprintf(
			"Create a professional product card for %s marketplace.\n"+
				"Show the product clearly and attractively.\n"+
				"The result must look like a finished e-commerce card, not a plain product photo.",
			marketplaceName,
		)
	}
}

// coverBlock задаёт требования для главной продающей карточки.
// Здесь мы отдельно запрещаем длинные заголовки и выдуманные буллеты, потому что они сильнее всего портят cover.
func coverBlock(marketplace string, p *ProductContext) string {
	if p == nil {
		return fmt.Sprintf(
			"Create the main selling product card for %s.\n"+
				"Show the product large, clean and visually attractive.\n"+
				"Use a strong marketplace cover composition.\n"+
				"Keep the product as the main focal point of the layout.\n"+
				"If a human model is present, the product must remain more visually important than the face.\n"+
				"Use one short Russian headline only.\n"+
				"Do not add benefit bullets or claims when they are not provided.\n"+
				"Do not invent brand names, product names or specific claims.\n"+
				"The card must look like a finished marketplace cover, not a plain product photo.",
			marketplace,
		)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Create the main selling product card for %s.\n", marketplace)
	sb.WriteString("Show the product large, clean and visually attractive.\n")
	sb.WriteString("Keep the product as the main focal point of the layout.\n")
	sb.WriteString("If a human model is present, the product must remain more visually important than the face.\n")
	fmt.Fprintf(&sb, "Use one short Russian headline based only on the provided product data: \"%s\".\n", p.Name)
	sb.WriteString("Do not turn the full input name into a long multi-line title if it hurts readability.\n")
	sb.WriteString("Do not mention any feature, material, fit, colorfastness or selling claim unless it is explicitly provided in the input.\n")

	if len(p.Benefits) > 0 {
		badges := p.Benefits[:min(3, len(p.Benefits))]
		fmt.Fprintf(&sb, "Add 2-3 short benefit badges: %s.\n", strings.Join(badges, ", "))
	} else {
		sb.WriteString("Do not add benefit bullets, badges or extra claims because no explicit benefits were provided.\n")
	}

	sb.WriteString("The card must look like a finished marketplace cover, not a plain product photo.")
	return sb.String()
}

// benefitsBlock задаёт правила для инфографики с преимуществами.
// Если benefits не пришли, мы запрещаем текстовые преимущества, чтобы модель не выдумывала их сама.
func benefitsBlock(marketplace string, p *ProductContext) string {
	if p == nil || len(p.Benefits) == 0 {
		return fmt.Sprintf(
			"Create a product benefits infographic for %s.\n"+
				"Use the source product photo as the main object.\n"+
				"Do not add benefit bullets or captions because explicit benefits were not provided.\n"+
				"Instead, use composition, crop, close-ups, icons or neutral visual callout zones without text.\n"+
				"The result must be a marketplace infographic, not a plain product photo.",
			marketplace,
		)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Create a product benefits infographic for %s.\n", marketplace)
	sb.WriteString("Use the source product photo as the main object.\n")
	sb.WriteString("Show benefits as short Russian captions:\n")
	for _, b := range p.Benefits {
		fmt.Fprintf(&sb, "- %s\n", b)
	}
	sb.WriteString("Use arrows, icons, callout blocks and clear visual hierarchy.")
	return sb.String()
}

// detailsBlock задаёт правила для карточки с деталями и характеристиками.
// Без характеристик блок остаётся визуальным, чтобы не провоцировать генератор на ложные материалы и цифры.
func detailsBlock(marketplace string, p *ProductContext) string {
	if p == nil || len(p.Characteristics) == 0 {
		return fmt.Sprintf(
			"Create a \"Детали товара\" card for %s.\n"+
				"Show close-up fragments of the product.\n"+
				"Add neutral visual callout areas without text and without inventing exact materials, numbers or technical claims.\n"+
				"The result must be marketplace infographic, not just a cleaned product photo.",
			marketplace,
		)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Create a \"Детали товара\" card for %s.\n", marketplace)
	sb.WriteString("Show close-up fragments of the product.\n")
	sb.WriteString("Add 3-5 callouts with short Russian labels.\n")
	sb.WriteString("Use characteristics where available:\n")
	for _, c := range p.Characteristics[:min(5, len(p.Characteristics))] {
		fmt.Fprintf(&sb, "- %s: %s\n", c.Name, c.Value)
	}
	sb.WriteString("The result must be marketplace infographic, not just a cleaned product photo.")
	return sb.String()
}

// dimensionsBlock задаёт правила для карточки с размерами.
// Числа можно показывать только если они есть во входных характеристиках.
func dimensionsBlock(marketplace string, p *ProductContext) string {
	chars := filterCharacteristicsByKeywords(p, "размер", "длина", "ширина", "высота", "глубина", "диаметр", "обхват")

	if len(chars) == 0 {
		return fmt.Sprintf(
			"Create a product dimensions card for %s.\n"+
				"Show the product on a clean background.\n"+
				"Add measurement lines, arrows and labels.\n"+
				"If no exact dimensions are provided, use neutral labels without numbers.\n"+
				"Do not invent dimensions, numbers or measurements.",
			marketplace,
		)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Create a product dimensions card for %s.\n", marketplace)
	sb.WriteString("Show the product on a clean background.\n")
	sb.WriteString("Add measurement lines, arrows and labels.\n")
	sb.WriteString("Use exact size-related characteristics only if they are present:\n")
	for _, c := range chars {
		fmt.Fprintf(&sb, "- %s: %s\n", c.Name, c.Value)
	}
	sb.WriteString("Do not invent dimensions, numbers or measurements.")
	return sb.String()
}

// compositionBlock задаёт правила для карточки с составом товара.
// Текст состава можно показывать только по входным характеристикам, иначе карточка остаётся нейтральной.
func compositionBlock(marketplace string, p *ProductContext) string {
	chars := filterCharacteristicsByKeywords(p, "состав", "материал", "ткань")

	if len(chars) == 0 {
		return fmt.Sprintf(
			"Create a product composition card for %s.\n"+
				"Show the product on a clean background with neat material-focused visual accents.\n"+
				"Do not add composition text, percentages or fabric names because they were not provided.\n"+
				"Use neutral visual callout zones without inventing materials or properties.",
			marketplace,
		)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Create a product composition card for %s.\n", marketplace)
	sb.WriteString("Show the product on a clean background.\n")
	sb.WriteString("Add 1-3 short Russian material callouts using only the provided composition facts:\n")
	for _, c := range chars[:min(3, len(chars))] {
		fmt.Fprintf(&sb, "- %s: %s\n", c.Name, c.Value)
	}
	sb.WriteString("Do not invent extra materials, percentages or textile properties.")
	return sb.String()
}

// useCaseBlock задаёт правила для карточки сценария использования.
// Отличается от usageBlock тем, что акцент делается на конкретном сценарии применения, а не на lifestyle.
func useCaseBlock(marketplace string, p *ProductContext) string {
	if p == nil || (p.Description == "" && p.Category == "") {
		return fmt.Sprintf(
			"Create a use case product card for %s.\n"+
				"Show the product solving a specific everyday problem or fitting a specific moment.\n"+
				"Keep the product recognizable and in focus.\n"+
				"Do not invent specific claims, brand names or scenarios.",
			marketplace,
		)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Create a use case product card for %s.\n", marketplace)
	sb.WriteString("Show the product in a specific, relatable usage scenario.\n")
	if p.Description != "" {
		fmt.Fprintf(&sb, "Base the scenario on this product context: %s.\n", p.Description)
	}
	if p.Category != "" {
		fmt.Fprintf(&sb, "Make the scenario relevant to category: %s.\n", p.Category)
	}
	sb.WriteString("Add one short Russian headline describing the situation.\n")
	sb.WriteString("Keep the product as the main visual element.")
	return sb.String()
}

// packagingBlock задаёт правила для карточки упаковки товара.
// Показываем упаковку как аргумент в пользу покупки: подарочная, защитная, экологичная и т.д.
func packagingBlock(marketplace string, p *ProductContext) string {
	packagingChars := filterCharacteristicsByKeywords(p, "упаковк", "коробк", "пакет", "комплект")

	if len(packagingChars) == 0 {
		return fmt.Sprintf(
			"Create a packaging card for %s.\n"+
				"Show the product packaging cleanly and attractively.\n"+
				"Emphasize quality, protection or gift-readiness without inventing specific packaging claims.\n"+
				"Do not add text labels or callouts unless packaging details were provided.",
			marketplace,
		)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Create a packaging card for %s.\n", marketplace)
	sb.WriteString("Show the product packaging cleanly and attractively.\n")
	sb.WriteString("Add 1-3 short Russian callouts based only on the provided packaging details:\n")
	for _, c := range packagingChars[:min(3, len(packagingChars))] {
		fmt.Fprintf(&sb, "- %s: %s\n", c.Name, c.Value)
	}
	sb.WriteString("Do not invent packaging claims beyond the provided data.")
	return sb.String()
}

// comparisonBlock задаёт правила для карточки сравнения товара с альтернативами.
// Цель — показать преимущества товара без явного упоминания конкурентов.
func comparisonBlock(marketplace string, p *ProductContext) string {
	if p == nil || len(p.Benefits) == 0 {
		return fmt.Sprintf(
			"Create a product comparison card for %s.\n"+
				"Use a visual \"before/after\" or \"us vs them\" layout without naming competitors.\n"+
				"Show the product as the superior choice through visual contrast and clean design.\n"+
				"Do not invent specific comparison claims or numbers.",
			marketplace,
		)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Create a product comparison card for %s.\n", marketplace)
	sb.WriteString("Use a visual comparison layout without naming competitors.\n")
	sb.WriteString("Show the product as the superior choice using these advantages:\n")
	for _, b := range p.Benefits[:min(4, len(p.Benefits))] {
		fmt.Fprintf(&sb, "- %s\n", b)
	}
	sb.WriteString("Keep the product visually dominant. Do not invent claims beyond the provided benefits.")
	return sb.String()
}

// filterCharacteristicsByKeywords отбирает только те характеристики, которые относятся к нужной теме карточки.
// Это помогает не подмешивать в prompt несвязанные поля из общего product context.
func filterCharacteristicsByKeywords(p *ProductContext, keywords ...string) []ProductCharacteristic {
	if p == nil || len(p.Characteristics) == 0 || len(keywords) == 0 {
		return nil
	}

	result := make([]ProductCharacteristic, 0, len(p.Characteristics))
	for _, c := range p.Characteristics {
		name := strings.ToLower(strings.TrimSpace(c.Name))
		for _, keyword := range keywords {
			if strings.Contains(name, keyword) {
				result = append(result, c)
				break
			}
		}
	}

	return result
}

// marketplaceDisplayName возвращает читаемое название marketplace для подстановки в шаблоны.
func marketplaceDisplayName(marketplaceID string) string {
	for _, m := range generateMarketplaces {
		if m.ID == marketplaceID {
			return m.Label
		}
	}
	return marketplaceID
}
