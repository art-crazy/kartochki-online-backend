package generation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kartochki-online-backend/internal/dbgen"
	"kartochki-online-backend/internal/projects"
)

type normalizedCreateInput struct {
	ProjectName    string
	MarketplaceID  string
	StyleID        string
	CardTypeIDs    []string
	CardCount      int
	SourceFileName string
	ModelID        string
	Product        *ProductContext
}

// validateCreateInput проверяет пользовательский запрос перед созданием проекта и generation.
// Здесь важно сверять source asset с user_id, чтобы нельзя было запустить генерацию с чужим файлом.
func (s *Service) validateCreateInput(ctx context.Context, input CreateInput) (uuid.UUID, uuid.UUID, normalizedCreateInput, error) {
	uid, err := uuid.Parse(strings.TrimSpace(input.UserID))
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, normalizedCreateInput{}, ErrSourceAssetNotFound
	}

	sourceAssetID, err := uuid.Parse(strings.TrimSpace(input.SourceAssetID))
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, normalizedCreateInput{}, ErrSourceAssetNotFound
	}

	marketplaceID := strings.TrimSpace(input.MarketplaceID)
	if !containsMarketplaceID(generateMarketplaces, marketplaceID) {
		return uuid.UUID{}, uuid.UUID{}, normalizedCreateInput{}, ErrInvalidMarketplace
	}

	styleID := strings.TrimSpace(input.StyleID)
	if !containsCatalogID(generateStyles, styleID) {
		return uuid.UUID{}, uuid.UUID{}, normalizedCreateInput{}, ErrInvalidStyle
	}

	// Разрешаем любой положительный card_count до общего лимита, чтобы фронтенд и API
	// не зависели от жёстко заданного списка вариантов.
	if input.CardCount <= 0 || input.CardCount > generateMaxCardCount {
		return uuid.UUID{}, uuid.UUID{}, normalizedCreateInput{}, ErrInvalidCardCount
	}

	cardTypeIDs := make([]string, 0, len(input.CardTypeIDs))
	seenCardTypes := make(map[string]struct{}, len(input.CardTypeIDs))
	for _, item := range input.CardTypeIDs {
		cardTypeID := strings.TrimSpace(item)
		if cardTypeID == "" || !containsCardTypeID(generateCardTypes, cardTypeID) {
			return uuid.UUID{}, uuid.UUID{}, normalizedCreateInput{}, ErrInvalidCardType
		}
		if _, exists := seenCardTypes[cardTypeID]; exists {
			continue
		}
		seenCardTypes[cardTypeID] = struct{}{}
		cardTypeIDs = append(cardTypeIDs, cardTypeID)
	}
	if len(cardTypeIDs) == 0 {
		return uuid.UUID{}, uuid.UUID{}, normalizedCreateInput{}, ErrInvalidCardType
	}

	// Если модель не задана, берём первую из каталога. Сейчас это дешёвый вариант по умолчанию.
	modelID := strings.TrimSpace(input.ModelID)
	if modelID == "" {
		modelID = generateModels[0].ID
	} else if !containsModelID(generateModels, modelID) {
		return uuid.UUID{}, uuid.UUID{}, normalizedCreateInput{}, ErrInvalidModel
	}

	projectName := strings.TrimSpace(input.ProjectName)
	if len(projectName) > projects.MaxProjectTitleLength {
		return uuid.UUID{}, uuid.UUID{}, normalizedCreateInput{}, ErrProjectNameTooLong
	}

	sourceAsset, err := s.queries.GetUserAssetByID(ctx, dbgen.GetUserAssetByIDParams{
		ID:     sourceAssetID,
		UserID: uid,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.UUID{}, uuid.UUID{}, normalizedCreateInput{}, ErrSourceAssetNotFound
		}
		return uuid.UUID{}, uuid.UUID{}, normalizedCreateInput{}, fmt.Errorf("get source asset by user: %w", err)
	}
	if sourceAsset.Kind != assetKindSourceImage {
		return uuid.UUID{}, uuid.UUID{}, normalizedCreateInput{}, ErrSourceAssetNotFound
	}

	product, err := validateProductContext(input.Product)
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, normalizedCreateInput{}, err
	}

	return uid, sourceAssetID, normalizedCreateInput{
		ProjectName:    projectName,
		MarketplaceID:  marketplaceID,
		StyleID:        styleID,
		CardTypeIDs:    cardTypeIDs,
		CardCount:      input.CardCount,
		SourceFileName: sourceAsset.OriginalFilename,
		ModelID:        modelID,
		Product:        product,
	}, nil
}

// validateProductContext проверяет поле product и возвращает нормализованный ProductContext.
// Если product nil, возвращает nil без ошибки — поле опциональное.
// Если product передан, name обязателен. Пустой объект без name считается ошибкой.
func validateProductContext(p *ProductContext) (*ProductContext, error) {
	if p == nil {
		return nil, nil
	}

	name := strings.TrimSpace(p.Name)
	if name == "" {
		return nil, ErrInvalidProduct
	}
	if len(name) > 200 {
		return nil, ErrInvalidProduct
	}

	category := strings.TrimSpace(p.Category)
	if len(category) > 120 {
		return nil, ErrInvalidProduct
	}

	brand := strings.TrimSpace(p.Brand)
	if len(brand) > 120 {
		return nil, ErrInvalidProduct
	}

	description := strings.TrimSpace(p.Description)
	if len(description) > 2000 {
		return nil, ErrInvalidProduct
	}

	if len(p.Benefits) > 10 {
		return nil, ErrInvalidProduct
	}
	benefits := make([]string, 0, len(p.Benefits))
	for _, b := range p.Benefits {
		trimmed := strings.TrimSpace(b)
		if len(trimmed) > 120 {
			return nil, ErrInvalidProduct
		}
		if trimmed != "" {
			benefits = append(benefits, trimmed)
		}
	}

	if len(p.Characteristics) > 20 {
		return nil, ErrInvalidProduct
	}
	characteristics := make([]ProductCharacteristic, 0, len(p.Characteristics))
	for _, c := range p.Characteristics {
		cName := strings.TrimSpace(c.Name)
		cValue := strings.TrimSpace(c.Value)
		if cName == "" || cValue == "" {
			return nil, ErrInvalidProduct
		}
		if len(cName) > 120 || len(cValue) > 120 {
			return nil, ErrInvalidProduct
		}
		characteristics = append(characteristics, ProductCharacteristic{Name: cName, Value: cValue})
	}

	return &ProductContext{
		Name:            name,
		Category:        category,
		Brand:           brand,
		Description:     description,
		Benefits:        benefits,
		Characteristics: characteristics,
	}, nil
}

func containsMarketplaceID(items []marketplaceOption, target string) bool {
	for _, item := range items {
		if item.ID == target {
			return true
		}
	}
	return false
}

func containsCatalogID(items []CatalogOption, target string) bool {
	for _, item := range items {
		if item.ID == target {
			return true
		}
	}
	return false
}

func containsCardTypeID(items []CardTypeOption, target string) bool {
	for _, item := range items {
		if item.ID == target {
			return true
		}
	}
	return false
}

func containsModelID(items []ModelOption, target string) bool {
	for _, item := range items {
		if item.ID == target {
			return true
		}
	}
	return false
}
