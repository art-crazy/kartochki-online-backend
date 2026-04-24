package generation

import (
	"fmt"
	"strings"
)

// BuildPrompt собирает prompt на основе премиального шаблона и выбора пользователя.
// Frontend влияет на marketplace, стиль, тип карточки и контекст товара, но не может
// заменить базовые правила сохранения исходного товара и премиальной композиции.
func BuildPrompt(input PromptInput) string {
	var sb strings.Builder

	writeBasePrompt(&sb, input)
	writeProductBlock(&sb, input.Product)
	writeMarketplaceBlock(&sb, input.MarketplaceID)
	writeStyleBlock(&sb, input.StyleID)
	writeCardTypeBlock(&sb, input.CardTypeID, input.Product)
	writeSafetyBlock(&sb, input.Product)

	return sb.String()
}

func writeBasePrompt(sb *strings.Builder, input PromptInput) {
	productName := "товар с входного изображения"
	if input.Product != nil && input.Product.Name != "" {
		productName = input.Product.Name
	}

	fmt.Fprintf(sb,
		"Создай премиальную карточку товара для маркетплейса уровня топ-брендов Wildberries / Ozon.\n"+
			"Товар: %s.\n"+
			"Используй входящее изображение как главный источник правды: сохрани тот же товар, форму, цвет, пропорции, фактуру и видимые детали.\n"+
			"Не заменяй товар на другой и не придумывай новую одежду.\n\n"+
			"СТИЛЬ: дорогой e-commerce, минимализм, clean UI, как premium бренд.\n"+
			"Это должна быть именно дизайнерская карточка товара, а не фотосессия и не обычное объявление.\n\n"+
			"КОМПОЗИЦИЯ:\n"+
			"- товар крупно, занимает около 70%% изображения;\n"+
			"- без модели, либо модель строго вторична и не перетягивает внимание;\n"+
			"- фон: мягкий градиент, например бежевый, серый или тёмный с глубиной;\n"+
			"- справа или слева аккуратные UI-блоки с преимуществами;\n"+
			"- 2-3 маленьких inset-блока с деталями товара, если они уместны.\n\n"+
			"ВИЗУАЛ:\n"+
			"- мягкий студийный свет;\n"+
			"- аккуратные тени;\n"+
			"- высокая детализация ткани и материалов;\n"+
			"- чистый фон;\n"+
			"- современный UI-дизайн.\n\n"+
			"ТИПОГРАФИКА:\n"+
			"- современный sans-serif;\n"+
			"- крупный заголовок;\n"+
			"- минимализм;\n"+
			"- весь текст на русском языке.\n",
		productName,
	)
}

func writeProductBlock(sb *strings.Builder, p *ProductContext) {
	if p == nil {
		sb.WriteString("\nДАННЫЕ О ТОВАРЕ:\n")
		sb.WriteString("- конкретные свойства товара не переданы;\n")
		sb.WriteString("- не добавляй бренд, состав, размеры, проценты и преимущества, которых нет на входном изображении или в prompt.\n")
		return
	}

	sb.WriteString("\nДАННЫЕ О ТОВАРЕ:\n")
	writePromptLine(sb, "Название", p.Name)
	writePromptLine(sb, "Категория", p.Category)
	writePromptLine(sb, "Бренд", p.Brand)
	writePromptLine(sb, "Описание", p.Description)

	if len(p.Benefits) > 0 {
		sb.WriteString("Преимущества, которые можно использовать короткими UI-блоками:\n")
		for _, benefit := range p.Benefits[:min(4, len(p.Benefits))] {
			fmt.Fprintf(sb, "- %s\n", benefit)
		}
	}

	if len(p.Characteristics) > 0 {
		sb.WriteString("Характеристики, которые можно использовать только если они подходят типу карточки:\n")
		for _, c := range p.Characteristics[:min(8, len(p.Characteristics))] {
			fmt.Fprintf(sb, "- %s: %s\n", c.Name, c.Value)
		}
	}
}

func writeMarketplaceBlock(sb *strings.Builder, marketplaceID string) {
	sb.WriteString("\nМАРКЕТПЛЕЙС:\n")
	switch marketplaceID {
	case "wildberries":
		sb.WriteString("- Wildberries;\n")
		sb.WriteString("- вертикальная карточка 3:4;\n")
		sb.WriteString("- текст должен читаться на мобильном экране.\n")
	case "ozon":
		sb.WriteString("- Ozon;\n")
		sb.WriteString("- вертикальная карточка 3:4;\n")
		sb.WriteString("- больше воздуха, чистые блоки, аккуратная сетка.\n")
	case "yandex_market":
		sb.WriteString("- Яндекс Маркет;\n")
		sb.WriteString("- квадратная карточка 1:1;\n")
		sb.WriteString("- спокойная композиция без перегруза.\n")
	default:
		sb.WriteString("- универсальная карточка маркетплейса;\n")
		sb.WriteString("- чистая композиция с понятной иерархией.\n")
	}
}

func writeStyleBlock(sb *strings.Builder, styleID string) {
	sb.WriteString("\nВЫБРАННЫЙ СТИЛЬ:\n")
	switch styleID {
	case "clean_catalog":
		sb.WriteString("- чистый каталог: светлый фон, минимум элементов, много воздуха;\n")
		sb.WriteString("- текст использовать только там, где он помогает понять товар.\n")
	case "accent_offer":
		sb.WriteString("- акцент на выгоде: более яркие UI-блоки, динамичная композиция, высокий контраст;\n")
		sb.WriteString("- сохранить премиальный вид, без дешёвых рекламных плашек.\n")
	case "premium_brand":
		sb.WriteString("- премиальный бренд: сдержанные цвета, дорогой editorial-вид, сильная типографика;\n")
		sb.WriteString("- один крупный заголовок и минимум вторичного текста.\n")
	default:
		sb.WriteString("- чистый профессиональный marketplace-дизайн.\n")
	}
}

func writeCardTypeBlock(sb *strings.Builder, cardTypeID string, p *ProductContext) {
	sb.WriteString("\nТИП КАРТОЧКИ:\n")
	switch cardTypeID {
	case "cover":
		sb.WriteString("- главная обложка;\n")
		sb.WriteString("- короткий заголовок 1-2 слова крупно;\n")
		sb.WriteString("- 2-3 коротких преимущества только из переданных данных.\n")
	case "benefits":
		sb.WriteString("- карточка преимуществ;\n")
		writeBenefitsInstruction(sb, p)
	case "details":
		sb.WriteString("- карточка деталей товара;\n")
		sb.WriteString("- покажи крупные фрагменты товара, ткань, швы, фактуру и аккуратные callout-блоки;\n")
		sb.WriteString("- подписи брать только из характеристик товара.\n")
	case "usage":
		sb.WriteString("- карточка сценария использования;\n")
		sb.WriteString("- покажи товар в реалистичном, но чистом контексте;\n")
		sb.WriteString("- товар должен оставаться главным объектом и быть узнаваемым.\n")
	case "dimensions":
		sb.WriteString("- карточка размеров;\n")
		writeDimensionsInstruction(sb, p)
	case "composition":
		sb.WriteString("- карточка состава или материала;\n")
		writeCompositionInstruction(sb, p)
	default:
		sb.WriteString("- универсальная премиальная карточка товара.\n")
	}
}

func writeBenefitsInstruction(sb *strings.Builder, p *ProductContext) {
	if p == nil || len(p.Benefits) == 0 {
		sb.WriteString("- явные преимущества не переданы, поэтому не выдумывай текстовые преимущества;\n")
		sb.WriteString("- используй нейтральные визуальные callout-зоны без конкретных claims.\n")
		return
	}

	sb.WriteString("- используй 3-4 очень коротких преимущества из списка:\n")
	for _, benefit := range p.Benefits[:min(4, len(p.Benefits))] {
		fmt.Fprintf(sb, "- %s\n", benefit)
	}
}

func writeDimensionsInstruction(sb *strings.Builder, p *ProductContext) {
	chars := filterCharacteristicsByKeywords(p, "размер", "длина", "ширина", "высота", "глубина", "диаметр", "обхват")
	if len(chars) == 0 {
		sb.WriteString("- точные размеры не переданы, поэтому не добавляй числа;\n")
		sb.WriteString("- можно использовать линии измерения без числовых значений.\n")
		return
	}

	sb.WriteString("- используй только эти размерные характеристики:\n")
	for _, c := range chars {
		fmt.Fprintf(sb, "- %s: %s\n", c.Name, c.Value)
	}
}

func writeCompositionInstruction(sb *strings.Builder, p *ProductContext) {
	chars := filterCharacteristicsByKeywords(p, "состав", "материал", "ткань")
	if len(chars) == 0 {
		sb.WriteString("- состав и материалы не переданы, поэтому не добавляй проценты и названия тканей;\n")
		sb.WriteString("- можно показать фактуру товара визуально без текстовых утверждений.\n")
		return
	}

	sb.WriteString("- используй только эти факты о составе или материале:\n")
	for _, c := range chars[:min(3, len(chars))] {
		fmt.Fprintf(sb, "- %s: %s\n", c.Name, c.Value)
	}
}

func writeSafetyBlock(sb *strings.Builder, p *ProductContext) {
	sb.WriteString("\nЗАПРЕТЫ И КАЧЕСТВО:\n")
	sb.WriteString("- никаких длинных предложений;\n")
	sb.WriteString("- никакой перегруженности;\n")
	sb.WriteString("- никакого маркетингового мусора;\n")
	sb.WriteString("- никаких случайных элементов;\n")
	sb.WriteString("- без дешёвого рекламного стиля;\n")
	sb.WriteString("- без кривого или нечитаемого текста;\n")
	sb.WriteString("- текст не должен закрывать товар;\n")
	sb.WriteString("- не добавляй фейковые скидки, рейтинги, сертификаты, бейджи и официальные отметки маркетплейса;\n")
	sb.WriteString("- не придумывай свойства товара, состав, размеры, бренд и цифры.\n")

	if p != nil && len(p.Benefits) > 0 {
		sb.WriteString("- преимущества можно брать только из блока данных о товаре.\n")
	}

	sb.WriteString("\nЦЕЛЬ: карточка должна выглядеть как дорогая карточка бренда с высоким CTR, а не как обычное объявление. Сделай визуал как у премиальных карточек одежды 2025-2026, с акцентом на стиль, свет и композицию.")
}

func writePromptLine(sb *strings.Builder, label, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fmt.Fprintf(sb, "%s: %s\n", label, value)
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
