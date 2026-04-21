package handlers

import (
	openapi "kartochki-online-backend/api/gen"
	"kartochki-online-backend/internal/generation"
)

// toGenerateMarketplaces конвертирует каталог marketplace в openapi-ответ.
func toGenerateMarketplaces(items []generation.CatalogOption) []openapi.GenerateMarketplace {
	result := make([]openapi.GenerateMarketplace, len(items))
	for i, item := range items {
		result[i] = openapi.GenerateMarketplace{
			Id:    item.ID,
			Label: item.Label,
		}
	}
	return result
}

// toGenerateStyles конвертирует каталог стилей в openapi-ответ.
func toGenerateStyles(items []generation.CatalogOption) []openapi.GenerateStyle {
	result := make([]openapi.GenerateStyle, len(items))
	for i, item := range items {
		result[i] = openapi.GenerateStyle{
			Id:    item.ID,
			Label: item.Label,
		}
	}
	return result
}

// toGenerateModels конвертирует список AI-моделей в openapi-ответ.
func toGenerateModels(items []generation.ModelOption) []openapi.GenerateModel {
	result := make([]openapi.GenerateModel, len(items))
	for i, item := range items {
		result[i] = openapi.GenerateModel{
			Id:            item.ID,
			Label:         item.Label,
			Description:   item.Description,
			PricePerImage: item.PricePerImage,
		}
	}
	return result
}

// toGenerateCardTypes конвертирует каталог типов карточек в openapi-ответ.
func toGenerateCardTypes(items []generation.CardTypeOption) []openapi.GenerateCardType {
	result := make([]openapi.GenerateCardType, len(items))
	for i, item := range items {
		ct := openapi.GenerateCardType{
			Id:    item.ID,
			Label: item.Label,
		}
		if item.DefaultSelected {
			ct.DefaultSelected = &item.DefaultSelected
		}
		result[i] = ct
	}
	return result
}

// toGeneratedCards конвертирует список готовых карточек в openapi-ответ.
func toGeneratedCards(items []generation.GeneratedCard) []openapi.GeneratedCard {
	result := make([]openapi.GeneratedCard, len(items))
	for i, item := range items {
		result[i] = openapi.GeneratedCard{
			Id:         mustParseUUID(item.ID),
			CardTypeId: item.CardTypeID,
			AssetId:    mustParseUUID(item.AssetID),
			PreviewUrl: item.PreviewURL,
		}
	}
	return result
}

// toGenerationStatusResponse конвертирует доменный Status в openapi.GenerationStatusResponse.
// Опциональные поля передаются как указатели — nil сериализуется с omitempty.
func toGenerationStatusResponse(result generation.Status) openapi.GenerationStatusResponse {
	resp := openapi.GenerationStatusResponse{
		GenerationId: mustParseUUID(result.GenerationID),
		Status:       openapi.GenerationStatusResponseStatus(result.Status),
	}
	if result.CurrentStep != "" {
		resp.CurrentStep = &result.CurrentStep
	}
	if result.ProgressPercent > 0 {
		resp.ProgressPercent = &result.ProgressPercent
	}
	if result.ErrorMessage != "" {
		resp.ErrorMessage = &result.ErrorMessage
	}
	if result.ArchiveDownloadURL != "" {
		resp.ArchiveDownloadUrl = &result.ArchiveDownloadURL
	}
	if len(result.ResultCards) > 0 {
		cards := toGeneratedCards(result.ResultCards)
		resp.ResultCards = &cards
	}
	return resp
}

// mapProductContext конвертирует опциональный openapi.ProductContext в доменный тип.
// Если поле не передано (nil), возвращает nil — генерация продолжается без контекста товара.
func mapProductContext(src *openapi.ProductContext) *generation.ProductContext {
	if src == nil {
		return nil
	}

	out := &generation.ProductContext{
		Name: src.Name,
	}
	if src.Category != nil {
		out.Category = *src.Category
	}
	if src.Brand != nil {
		out.Brand = *src.Brand
	}
	if src.Description != nil {
		out.Description = *src.Description
	}
	if src.Benefits != nil {
		out.Benefits = *src.Benefits
	}
	if src.Characteristics != nil {
		chars := make([]generation.ProductCharacteristic, len(*src.Characteristics))
		for i, c := range *src.Characteristics {
			chars[i] = generation.ProductCharacteristic{Name: c.Name, Value: c.Value}
		}
		out.Characteristics = chars
	}
	return out
}
