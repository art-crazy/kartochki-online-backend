package generation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kartochki-online-backend/internal/dbgen"
	"kartochki-online-backend/internal/platform/storage"
)

// HandleGeneration обрабатывает фоновую задачу и переводит generation в итоговое состояние.
func (s *Service) HandleGeneration(ctx context.Context, generationIDValue string) error {
	generationID, err := uuid.Parse(strings.TrimSpace(generationIDValue))
	if err != nil {
		return fmt.Errorf("parse generation id: %w", err)
	}

	if rows, err := s.queries.MarkGenerationProcessing(ctx, dbgen.MarkGenerationProcessingParams{
		ID:              generationID,
		CurrentStep:     "preparing",
		ProgressPercent: 5,
	}); err != nil {
		return fmt.Errorf("mark generation processing: %w", err)
	} else if rows == 0 {
		return nil
	}

	if err := s.processGeneration(ctx, generationID); err != nil {
		if cleanupErr := s.cleanupFailedGeneration(ctx, generationID); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("cleanup failed generation: %w", cleanupErr))
		}
		_ = s.queries.MarkGenerationFailed(ctx, dbgen.MarkGenerationFailedParams{
			ID:           generationID,
			ErrorMessage: trimErrorMessage(err),
		})
		return err
	}

	return nil
}

// cleanupFailedGeneration удаляет частично созданные файлы и записи.
// Это делает повторный запуск пользователя независимым от мусора после неудачной попытки.
func (s *Service) cleanupFailedGeneration(ctx context.Context, generationID uuid.UUID) error {
	row, err := s.queries.GetGenerationByID(ctx, generationID)
	if err != nil {
		return fmt.Errorf("get generation before cleanup: %w", err)
	}

	cards, err := s.queries.ListGeneratedCardsByGenerationID(ctx, generationID)
	if err != nil {
		return fmt.Errorf("list generated cards before cleanup: %w", err)
	}

	// Сначала убираем строки связей, потом удаляем asset-записи и файлы.
	// Так cleanup не зависит от каскадного поведения внешних ключей.
	if len(cards) > 0 {
		if err := s.queries.DeleteGeneratedCardsByGenerationID(ctx, generationID); err != nil {
			return fmt.Errorf("delete generated card rows: %w", err)
		}
	}

	for _, card := range cards {
		if err := s.storage.Delete(ctx, card.StorageKey); err != nil {
			return fmt.Errorf("delete generated card file: %w", err)
		}

		if _, err := s.queries.DeleteAssetByID(ctx, card.AssetID); err != nil {
			return fmt.Errorf("delete generated card asset: %w", err)
		}
	}

	if row.ArchiveAssetID.Valid {
		archiveAsset, err := s.queries.GetAssetByID(ctx, row.ArchiveAssetID.Bytes)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("get archive asset before cleanup: %w", err)
		}

		if err == nil {
			if err := s.storage.Delete(ctx, archiveAsset.StorageKey); err != nil {
				return fmt.Errorf("delete archive file: %w", err)
			}
		}

		if err := s.queries.ClearGenerationArchiveAsset(ctx, generationID); err != nil {
			return fmt.Errorf("clear generation archive reference: %w", err)
		}

		if _, err := s.queries.DeleteAssetByID(ctx, row.ArchiveAssetID.Bytes); err != nil {
			return fmt.Errorf("delete archive asset: %w", err)
		}
	}

	return nil
}

func (s *Service) processGeneration(ctx context.Context, generationID uuid.UUID) error {
	generationRow, err := s.queries.GetGenerationByID(ctx, generationID)
	if err != nil {
		return fmt.Errorf("get generation for processing: %w", err)
	}
	sourceAsset, sourceImageBody, err := s.loadSourceImage(ctx, generationRow.SourceAssetID)
	if err != nil {
		return err
	}

	cardTypes, err := s.queries.ListGenerationCardTypes(ctx, generationID)
	if err != nil {
		return fmt.Errorf("list generation card types: %w", err)
	}
	if len(cardTypes) == 0 {
		return fmt.Errorf("generation has no card types")
	}

	// Читаем контекст товара до UpdateGenerationProgress, чтобы не записывать прогресс
	// если данные о товаре повреждены или недоступны — статус в этом случае сразу пойдёт в failed.
	productCtx, err := s.loadProductContext(ctx, generationID)
	if err != nil {
		return err
	}

	if _, err := s.queries.UpdateGenerationProgress(ctx, dbgen.UpdateGenerationProgressParams{
		ID:              generationID,
		CurrentStep:     "rendering_cards",
		ProgressPercent: 20,
	}); err != nil {
		return fmt.Errorf("update generation progress before rendering: %w", err)
	}

	archiveFiles := make([]storage.ArchiveFile, 0, generationRow.CardCount)
	aspectRatio := marketplaceAspectRatio(generationRow.MarketplaceID)
	for i := 0; i < int(generationRow.CardCount); i++ {
		cardType := cardTypes[i%len(cardTypes)]

		// Собираем prompt через новый builder с учётом marketplace, стиля, типа карточки и контекста товара.
		prompt := BuildPrompt(PromptInput{
			MarketplaceID: generationRow.MarketplaceID,
			StyleID:       generationRow.StyleID,
			CardTypeID:    cardType.CardTypeID,
			Product:       productCtx,
		})
		imgBytes, err := s.imageGenerator.GenerateImage(ctx, ImageGenerateInput{
			Prompt:              prompt,
			SourceImageBody:     sourceImageBody,
			SourceImageMIMEType: sourceAsset.MimeType,
			AspectRatio:         aspectRatio,
			ModelID:             generationRow.ModelID,
		})
		if err != nil {
			return fmt.Errorf("generate card image (step %d, type %s): %w", i+1, cardType.CardTypeID, err)
		}

		fileName := fmt.Sprintf("%02d-%s.png", i+1, sanitizeFileSegment(cardType.CardTypeID))
		targetKey := filepath.ToSlash(filepath.Join("generated", generationID.String(), fileName))

		savedFile, err := s.storage.Save(ctx, targetKey, imgBytes)
		if err != nil {
			return fmt.Errorf("save generated card image: %w", err)
		}

		cardAssetID := uuid.New()
		if err := s.persistGeneratedCard(ctx, cardAssetID, generationRow, cardType.CardTypeID, int32(i), savedFile); err != nil {
			_ = s.storage.Delete(ctx, savedFile.StorageKey)
			return err
		}

		archiveFiles = append(archiveFiles, storage.ArchiveFile{
			Name:       fileName,
			StorageKey: savedFile.StorageKey,
		})

		progress := 20 + ((i + 1) * 60 / int(generationRow.CardCount))
		if _, err := s.queries.UpdateGenerationProgress(ctx, dbgen.UpdateGenerationProgressParams{
			ID:              generationID,
			CurrentStep:     "rendering_cards",
			ProgressPercent: int32(progress),
		}); err != nil {
			return fmt.Errorf("update generation progress during rendering: %w", err)
		}
	}

	if _, err := s.queries.UpdateGenerationProgress(ctx, dbgen.UpdateGenerationProgressParams{
		ID:              generationID,
		CurrentStep:     "packing_archive",
		ProgressPercent: 90,
	}); err != nil {
		return fmt.Errorf("update generation progress before archive: %w", err)
	}

	archiveKey := filepath.ToSlash(filepath.Join("archives", generationID.String(), "cards.zip"))
	archiveFile, err := s.storage.CreateZIP(ctx, archiveKey, archiveFiles)
	if err != nil {
		return fmt.Errorf("create generation archive: %w", err)
	}

	archiveAssetID := uuid.New()
	if err := s.persistArchiveAsset(ctx, archiveAssetID, generationRow, archiveFile); err != nil {
		_ = s.storage.Delete(ctx, archiveFile.StorageKey)
		return err
	}

	if err := s.queries.MarkGenerationCompleted(ctx, dbgen.MarkGenerationCompletedParams{
		ID: generationID,
		ArchiveAssetID: pgtype.UUID{
			Bytes: archiveAssetID,
			Valid: true,
		},
	}); err != nil {
		_ = s.storage.Delete(ctx, archiveFile.StorageKey)
		_, _ = s.queries.DeleteAssetByID(ctx, archiveAssetID)
		return fmt.Errorf("mark generation completed: %w", err)
	}

	if _, err := s.queries.ActivateProjectByID(ctx, generationRow.ProjectID); err != nil {
		return fmt.Errorf("activate project after generation: %w", err)
	}

	return nil
}

// loadProductContext читает контекст товара для generation из БД.
// Если контекст не найден — это нормальная ситуация, метод возвращает nil без ошибки.
func (s *Service) loadProductContext(ctx context.Context, generationID uuid.UUID) (*ProductContext, error) {
	row, err := s.queries.GetGenerationProductContextByGenerationID(ctx, generationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("load product context: %w", err)
	}

	p := &ProductContext{
		Name:     row.Name,
		Benefits: row.Benefits,
	}
	if row.Category.Valid {
		p.Category = row.Category.String
	}
	if row.Brand.Valid {
		p.Brand = row.Brand.String
	}
	if row.Description.Valid {
		p.Description = row.Description.String
	}
	if len(row.Characteristics) > 0 {
		if err := json.Unmarshal(row.Characteristics, &p.Characteristics); err != nil {
			return nil, fmt.Errorf("unmarshal product characteristics: %w", err)
		}
	}

	return p, nil
}

// loadSourceImage загружает исходник generation из БД и storage.
// Дополнительная проверка kind защищает worker от неконсистентных данных в очереди или БД.
func (s *Service) loadSourceImage(ctx context.Context, assetID uuid.UUID) (dbgen.Asset, []byte, error) {
	sourceAsset, err := s.queries.GetAssetByID(ctx, assetID)
	if err != nil {
		return dbgen.Asset{}, nil, fmt.Errorf("get source asset for processing: %w", err)
	}
	if sourceAsset.Kind != assetKindSourceImage {
		return dbgen.Asset{}, nil, fmt.Errorf("asset %s is not a source image", assetID)
	}

	// Читаем исходник один раз до цикла, чтобы каждая карточка опиралась на одно и то же фото товара.
	sourceImageBody, err := s.storage.Read(ctx, sourceAsset.StorageKey)
	if err != nil {
		return dbgen.Asset{}, nil, fmt.Errorf("read source image for processing: %w", err)
	}

	return sourceAsset, sourceImageBody, nil
}

// noopImageGenerator используется, когда ROUTERAI_API_KEY не задан.
// Он явно валит задачу, чтобы система не создавала пустые файлы.
type noopImageGenerator struct{}

func (noopImageGenerator) GenerateImage(_ context.Context, _ ImageGenerateInput) ([]byte, error) {
	return nil, fmt.Errorf("image generator is not configured (ROUTERAI_API_KEY is not set)")
}
