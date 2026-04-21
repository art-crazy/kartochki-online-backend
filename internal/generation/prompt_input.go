package generation

// PromptInput описывает все данные, которые prompt builder использует для сборки одного prompt.
// Product может быть nil, тогда шаблоны строятся без конкретных деталей товара.
type PromptInput struct {
	MarketplaceID string
	StyleID       string
	CardTypeID    string
	Product       *ProductContext
}
