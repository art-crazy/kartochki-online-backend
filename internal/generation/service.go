package generation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"kartochki-online-backend/internal/dbgen"
	"kartochki-online-backend/internal/jobs"
	"kartochki-online-backend/internal/platform/storage"
	"kartochki-online-backend/internal/projects"
)

// ImageGenerateInput описывает параметры одной карточки, которую нужно сгенерировать.
type ImageGenerateInput struct {
	// Prompt — текстовое описание желаемого изображения.
	Prompt string
	// AspectRatio — соотношение сторон в формате "W:H", например "3:4".
	AspectRatio string
	// ModelID — идентификатор модели в RouterAI, например "google/gemini-2.5-flash-image".
	ModelID string
}

// ImageGenerator — контракт для любого провайдера генерации изображений.
// Реализует internal/platform/routerai.Client; при отсутствии ключа используется noopImageGenerator.
type ImageGenerator interface {
	GenerateImage(ctx context.Context, input ImageGenerateInput) ([]byte, error)
}

const (
	defaultProjectTitle = "Новый проект"
)

var (
	// generateMarketplaces хранит маркетплейсы вместе с aspect ratio и частью промпта.
	// AspectRatio используется воркером при генерации — добавление маркетплейса не требует правок в других местах.
	generateMarketplaces = []marketplaceOption{
		{CatalogOption: CatalogOption{ID: "wildberries", Label: "Wildberries", PromptPart: "Wildberries"}, AspectRatio: "3:4"},
		{CatalogOption: CatalogOption{ID: "ozon", Label: "Ozon", PromptPart: "Ozon"}, AspectRatio: "3:4"},
		{CatalogOption: CatalogOption{ID: "yandex_market", Label: "Яндекс Маркет", PromptPart: "Yandex Market"}, AspectRatio: "1:1"},
	}
	generateStyles = []CatalogOption{
		{ID: "clean_catalog", Label: "Чистый каталог", PromptPart: "clean catalog, white background, minimal"},
		{ID: "accent_offer", Label: "Акцент на выгоде", PromptPart: "accent on offer and benefits, vibrant, attention-grabbing"},
		{ID: "premium_brand", Label: "Премиальный бренд", PromptPart: "premium brand, luxury, elegant, sophisticated"},
	}
	generateCardTypes = []CardTypeOption{
		{ID: "cover", Label: "Обложка", DefaultSelected: true, PromptPart: "main product cover shot, hero image"},
		{ID: "benefits", Label: "Преимущества", DefaultSelected: true, PromptPart: "product benefits and key features highlighted"},
		{ID: "details", Label: "Детали", DefaultSelected: true, PromptPart: "product details and close-up view"},
		{ID: "usage", Label: "Сценарий использования", PromptPart: "product in use, lifestyle scenario"},
		{ID: "dimensions", Label: "Размеры", PromptPart: "product dimensions and measurements diagram"},
		{ID: "composition", Label: "Состав", PromptPart: "product composition and materials"},
	}
	generateCardCountOptions = []int{3, 5, 7, 10}

	// generateModels — список AI-моделей, доступных для выбора при генерации.
	// PricePerImage — стоимость одного изображения в копейках (целое число, без float).
	// Эти цены ориентировочны и должны отражать реальную стоимость у провайдера RouterAI.
	// Первая модель в списке считается выбором по умолчанию.
	generateModels = []ModelOption{
		{
			ID:            "google/gemini-2.5-flash-image",
			Label:         "Gemini 2.5 Flash",
			Description:   "Быстро и дёшево. Хорошо для прототипирования и большого количества карточек.",
			PricePerImage: 5,
		},
		{
			ID:            "google/gemini-3-pro-image-preview",
			Label:         "Gemini 3 Pro",
			Description:   "Высокое качество изображений, детальная проработка. Оптимален для финального результата.",
			PricePerImage: 25,
		},
		{
			ID:            "black-forest-labs/flux.2-pro",
			Label:         "FLUX.2 Pro",
			Description:   "Фотореалистичные изображения. Лучший выбор для товаров с реалистичной фотографией.",
			PricePerImage: 40,
		},
		{
			ID:            "openai/gpt-5-image-mini",
			Label:         "GPT-5 Image Mini",
			Description:   "Хорошее соотношение цены и качества от OpenAI.",
			PricePerImage: 15,
		},
	}
)

// CatalogOption описывает один доступный вариант из каталога generation-конфига.
type CatalogOption struct {
	ID         string
	Label      string
	PromptPart string // часть промпта для AI; не передаётся на фронтенд
}

// CardTypeOption описывает один тип карточки для страницы генерации.
type CardTypeOption struct {
	ID              string
	Label           string
	DefaultSelected bool
	PromptPart      string // часть промпта для AI; не передаётся на фронтенд
}

// marketplaceOption расширяет CatalogOption полем AspectRatio для воркера генерации.
// Хранится внутри пакета — на фронтенд уходит только CatalogOption.
type marketplaceOption struct {
	CatalogOption
	AspectRatio string
}

// ModelOption описывает одну AI-модель, доступную для выбора пользователем.
// PricePerImage — стоимость одного изображения в копейках.
// Фронтенд умножает её на CardCount, чтобы показать итоговую стоимость до запуска генерации.
type ModelOption struct {
	ID            string
	Label         string
	Description   string
	PricePerImage int
}

// Config описывает справочные данные для страницы `/app/generate`.
type Config struct {
	Marketplaces     []CatalogOption
	Styles           []CatalogOption
	CardTypes        []CardTypeOption
	CardCountOptions []int
	Models           []ModelOption
}

// UploadedImage описывает исходное изображение, которое frontend прислал через upload endpoint.
type UploadedImage struct {
	FileName    string
	ContentType string
	Body        []byte
}

// UploadedAsset описывает результат сохранения исходного изображения.
type UploadedAsset struct {
	AssetID    string
	PreviewURL string
}

// CreateInput описывает запуск новой генерации карточек.
type CreateInput struct {
	UserID        string
	ProjectName   string
	MarketplaceID string
	StyleID       string
	CardTypeIDs   []string
	CardCount     int
	SourceAssetID string
	// ModelID — идентификатор AI-модели из каталога generateModels.
	// Если пустой, используется первая модель из каталога.
	ModelID string
}

// CreatedGeneration описывает результат постановки генерации в очередь.
type CreatedGeneration struct {
	GenerationID string
	Status       string
}

// GeneratedCard описывает одну готовую карточку в ответе polling endpoint.
type GeneratedCard struct {
	ID         string
	CardTypeID string
	AssetID    string
	PreviewURL string
}

// Status описывает текущее состояние генерации и уже доступные артефакты.
type Status struct {
	GenerationID       string
	Status             string
	CurrentStep        string
	ProgressPercent    int
	ErrorMessage       string
	ArchiveDownloadURL string
	ResultCards        []GeneratedCard
}

// Service управляет generation-сценарием поверх sqlc, storage и очереди фоновых задач.
type Service struct {
	pool           *pgxpool.Pool
	queries        *dbgen.Queries
	jobsClient     *jobs.Client
	storage        generationStorage
	limits         generationLimits
	imageGenerator ImageGenerator
}

type generationStorage interface {
	Save(ctx context.Context, storageKey string, body []byte) (storage.SavedFile, error)
	CreateZIP(ctx context.Context, targetKey string, files []storage.ArchiveFile) (storage.SavedFile, error)
	Delete(ctx context.Context, storageKey string) error
	PublicURL(storageKey string) string
}

type generationLimits interface {
	EnsureGenerationAllowed(ctx context.Context, userID string, requestedCards int) error
}

// NewService создаёт сервис generation и связывает БД с локальным storage и очередью.
// imageGenerator может быть nil — тогда используется noopImageGenerator, который возвращает ошибку.
func NewService(pool *pgxpool.Pool, queries *dbgen.Queries, jobsClient *jobs.Client, storage generationStorage, limits generationLimits, imageGenerator ImageGenerator) *Service {
	gen := imageGenerator
	if gen == nil {
		gen = noopImageGenerator{}
	}
	return &Service{
		pool:           pool,
		queries:        queries,
		jobsClient:     jobsClient,
		storage:        storage,
		limits:         limits,
		imageGenerator: gen,
	}
}

// GetConfig возвращает каталог вариантов для страницы `/app/generate`.
func (s *Service) GetConfig(_ context.Context) Config {
	return Config{
		Marketplaces:     marketplacesToCatalog(generateMarketplaces),
		Styles:           cloneCatalogOptions(generateStyles),
		CardTypes:        cloneCardTypeOptions(generateCardTypes),
		CardCountOptions: append([]int(nil), generateCardCountOptions...),
		Models:           cloneModelOptions(generateModels),
	}
}

// UploadSourceImage сохраняет исходник пользователя и возвращает id asset для следующего шага.
func (s *Service) UploadSourceImage(ctx context.Context, userID string, image UploadedImage) (UploadedAsset, error) {
	uid, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return UploadedAsset{}, ErrSourceAssetNotFound
	}

	image = normalizeUploadedImage(image)
	if len(image.Body) == 0 {
		return UploadedAsset{}, ErrImageRequired
	}

	ext, mimeType, err := normalizeImageType(image.FileName, image.ContentType)
	if err != nil {
		return UploadedAsset{}, err
	}

	assetID := uuid.New()
	storageKey := filepath.ToSlash(filepath.Join("uploads", uid.String(), assetID.String()+ext))
	saved, err := s.storage.Save(ctx, storageKey, image.Body)
	if err != nil {
		return UploadedAsset{}, fmt.Errorf("save uploaded image: %w", err)
	}

	row, err := s.queries.CreateAsset(ctx, dbgen.CreateAssetParams{
		ID:               assetID,
		UserID:           uid,
		Kind:             "source_image",
		StorageKey:       saved.StorageKey,
		OriginalFilename: image.FileName,
		MimeType:         mimeType,
		SizeBytes:        saved.SizeBytes,
	})
	if err != nil {
		_ = s.storage.Delete(ctx, storageKey)
		return UploadedAsset{}, fmt.Errorf("create uploaded asset: %w", err)
	}

	return UploadedAsset{
		AssetID:    row.ID.String(),
		PreviewURL: s.storage.PublicURL(row.StorageKey),
	}, nil
}

// Create создаёт проект, запись генерации и ставит фоновую задачу в очередь.
func (s *Service) Create(ctx context.Context, input CreateInput) (CreatedGeneration, error) {
	uid, sourceAssetID, normalized, err := s.validateCreateInput(ctx, input)
	if err != nil {
		return CreatedGeneration{}, err
	}
	if s.limits != nil {
		if err := s.limits.EnsureGenerationAllowed(ctx, uid.String(), normalized.CardCount); err != nil {
			return CreatedGeneration{}, err
		}
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CreatedGeneration{}, fmt.Errorf("begin generation create tx: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)
	projectRow, err := txQueries.CreateProject(ctx, dbgen.CreateProjectParams{
		UserID:             uid,
		Title:              buildProjectTitle(normalized.ProjectName, normalized.SourceFileName),
		Marketplace:        normalized.MarketplaceID,
		ProductName:        "",
		ProductDescription: "",
	})
	if err != nil {
		return CreatedGeneration{}, fmt.Errorf("create project for generation: %w", err)
	}

	generationID := uuid.New()
	row, err := txQueries.CreateGeneration(ctx, dbgen.CreateGenerationParams{
		ID:            generationID,
		UserID:        uid,
		ProjectID:     projectRow.ID,
		SourceAssetID: sourceAssetID,
		MarketplaceID: normalized.MarketplaceID,
		StyleID:       normalized.StyleID,
		CardCount:     int32(normalized.CardCount),
		ModelID:       normalized.ModelID,
	})
	if err != nil {
		return CreatedGeneration{}, fmt.Errorf("create generation row: %w", err)
	}

	for i, cardTypeID := range normalized.CardTypeIDs {
		if err := txQueries.AddGenerationCardType(ctx, dbgen.AddGenerationCardTypeParams{
			GenerationID: generationID,
			Position:     int32(i),
			CardTypeID:   cardTypeID,
		}); err != nil {
			return CreatedGeneration{}, fmt.Errorf("add generation card type: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return CreatedGeneration{}, fmt.Errorf("commit generation create tx: %w", err)
	}

	if s.jobsClient == nil {
		_ = s.queries.MarkGenerationFailed(ctx, dbgen.MarkGenerationFailedParams{
			ID:           generationID,
			ErrorMessage: "background queue is not configured",
		})
		return CreatedGeneration{}, fmt.Errorf("generation queue is not configured")
	}

	if _, err := s.jobsClient.EnqueueGeneration(ctx, jobs.GenerationPayload{
		GenerationID: generationID.String(),
	}); err != nil {
		_ = s.queries.MarkGenerationFailed(ctx, dbgen.MarkGenerationFailedParams{
			ID:           generationID,
			ErrorMessage: "failed to enqueue generation",
		})
		return CreatedGeneration{}, fmt.Errorf("enqueue generation: %w", err)
	}

	return CreatedGeneration{
		GenerationID: row.ID.String(),
		Status:       row.Status,
	}, nil
}

// GetByID возвращает статус генерации и готовые артефакты, если они уже появились.
func (s *Service) GetByID(ctx context.Context, userID string, generationID string) (Status, error) {
	uid, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return Status{}, ErrGenerationNotFound
	}

	gid, err := uuid.Parse(strings.TrimSpace(generationID))
	if err != nil {
		return Status{}, ErrGenerationNotFound
	}

	row, err := s.queries.GetUserGenerationByID(ctx, dbgen.GetUserGenerationByIDParams{
		ID:     gid,
		UserID: uid,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Status{}, ErrGenerationNotFound
		}
		return Status{}, fmt.Errorf("get generation by id: %w", err)
	}

	result := Status{
		GenerationID:    row.ID.String(),
		Status:          row.Status,
		CurrentStep:     strings.TrimSpace(row.CurrentStep),
		ProgressPercent: int(row.ProgressPercent),
		ErrorMessage:    strings.TrimSpace(row.ErrorMessage),
	}

	if strings.TrimSpace(row.ArchiveStorageKey) != "" {
		result.ArchiveDownloadURL = s.storage.PublicURL(row.ArchiveStorageKey)
	}

	if row.Status != "completed" {
		return result, nil
	}

	cards, err := s.queries.ListGeneratedCardsByGenerationID(ctx, gid)
	if err != nil {
		return Status{}, fmt.Errorf("list generated cards: %w", err)
	}

	result.ResultCards = make([]GeneratedCard, len(cards))
	for i, card := range cards {
		result.ResultCards[i] = GeneratedCard{
			ID:         card.ID.String(),
			CardTypeID: card.CardTypeID,
			AssetID:    card.AssetID.String(),
			PreviewURL: s.storage.PublicURL(card.StorageKey),
		}
	}

	return result, nil
}

// HandleGeneration обрабатывает задачу Asynq и переводит generation в итоговое состояние.
func (s *Service) HandleGeneration(ctx context.Context, payload jobs.GenerationPayload) error {
	generationID, err := uuid.Parse(strings.TrimSpace(payload.GenerationID))
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

// cleanupFailedGeneration удаляет частично созданные файлы и записи,
// чтобы повторный запуск пользователя не упирался в мусор от неудачной попытки.
func (s *Service) cleanupFailedGeneration(ctx context.Context, generationID uuid.UUID) error {
	row, err := s.queries.GetGenerationByID(ctx, generationID)
	if err != nil {
		return fmt.Errorf("get generation before cleanup: %w", err)
	}

	cards, err := s.queries.ListGeneratedCardsByGenerationID(ctx, generationID)
	if err != nil {
		return fmt.Errorf("list generated cards before cleanup: %w", err)
	}

	// Сначала убираем строки связей, а потом удаляем asset-записи.
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

	cardTypes, err := s.queries.ListGenerationCardTypes(ctx, generationID)
	if err != nil {
		return fmt.Errorf("list generation card types: %w", err)
	}
	if len(cardTypes) == 0 {
		return fmt.Errorf("generation has no card types")
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

		// Генерируем изображение через AI-провайдер.
		// Промпт составляется по параметрам генерации: маркетплейс, стиль, тип карточки.
		prompt := buildCardPrompt(generationRow.MarketplaceID, generationRow.StyleID, cardType.CardTypeID)
		imgBytes, err := s.imageGenerator.GenerateImage(ctx, ImageGenerateInput{
			Prompt:      prompt,
			AspectRatio: aspectRatio,
			ModelID:     generationRow.ModelID,
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
		return fmt.Errorf("mark generation completed: %w", err)
	}

	if _, err := s.queries.ActivateProjectByID(ctx, generationRow.ProjectID); err != nil {
		return fmt.Errorf("activate project after generation: %w", err)
	}

	return nil
}

type normalizedCreateInput struct {
	ProjectName    string
	MarketplaceID  string
	StyleID        string
	CardTypeIDs    []string
	CardCount      int
	SourceFileName string
	ModelID        string
}

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

	if !containsInt(generateCardCountOptions, input.CardCount) {
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

	// Если модель не задана — берём первую из каталога (дешёвую по умолчанию).
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
	if sourceAsset.Kind != "source_image" {
		return uuid.UUID{}, uuid.UUID{}, normalizedCreateInput{}, ErrSourceAssetNotFound
	}

	return uid, sourceAssetID, normalizedCreateInput{
		ProjectName:    projectName,
		MarketplaceID:  marketplaceID,
		StyleID:        styleID,
		CardTypeIDs:    cardTypeIDs,
		CardCount:      input.CardCount,
		SourceFileName: sourceAsset.OriginalFilename,
		ModelID:        modelID,
	}, nil
}

func (s *Service) persistGeneratedCard(
	ctx context.Context,
	assetID uuid.UUID,
	generationRow dbgen.Generation,
	cardTypeID string,
	position int32,
	savedFile storage.SavedFile,
) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin generated card tx: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)
	if _, err := txQueries.CreateAsset(ctx, dbgen.CreateAssetParams{
		ID:               assetID,
		UserID:           generationRow.UserID,
		Kind:             "generated_card",
		StorageKey:       savedFile.StorageKey,
		OriginalFilename: filepath.Base(savedFile.StorageKey),
		// RouterAI всегда возвращает PNG независимо от формата исходника.
		MimeType:  "image/png",
		SizeBytes: savedFile.SizeBytes,
	}); err != nil {
		return fmt.Errorf("create generated asset: %w", err)
	}

	if _, err := txQueries.CreateGeneratedCard(ctx, dbgen.CreateGeneratedCardParams{
		GenerationID: generationRow.ID,
		AssetID:      assetID,
		CardTypeID:   cardTypeID,
		Position:     position,
	}); err != nil {
		return fmt.Errorf("create generated card row: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit generated card tx: %w", err)
	}

	return nil
}

func (s *Service) persistArchiveAsset(ctx context.Context, assetID uuid.UUID, generationRow dbgen.Generation, archiveFile storage.SavedFile) error {
	if _, err := s.queries.CreateAsset(ctx, dbgen.CreateAssetParams{
		ID:               assetID,
		UserID:           generationRow.UserID,
		Kind:             "archive",
		StorageKey:       archiveFile.StorageKey,
		OriginalFilename: "cards.zip",
		MimeType:         "application/zip",
		SizeBytes:        archiveFile.SizeBytes,
	}); err != nil {
		return fmt.Errorf("create archive asset: %w", err)
	}

	return nil
}

func buildProjectTitle(projectName string, sourceFileName string) string {
	projectName = strings.TrimSpace(projectName)
	if projectName != "" {
		return projectName
	}

	base := strings.TrimSpace(strings.TrimSuffix(sourceFileName, filepath.Ext(sourceFileName)))
	if base == "" {
		return defaultProjectTitle
	}
	if len(base) > projects.MaxProjectTitleLength {
		return base[:projects.MaxProjectTitleLength]
	}
	return base
}

func normalizeUploadedImage(image UploadedImage) UploadedImage {
	image.FileName = strings.TrimSpace(image.FileName)
	image.ContentType = strings.TrimSpace(strings.ToLower(image.ContentType))
	return image
}

func normalizeImageType(fileName string, contentType string) (string, string, error) {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(fileName)))
	switch {
	case contentType == "image/png" || ext == ".png":
		return ".png", "image/png", nil
	case contentType == "image/jpeg" || contentType == "image/jpg" || ext == ".jpg" || ext == ".jpeg":
		return ".jpg", "image/jpeg", nil
	case contentType == "image/webp" || ext == ".webp":
		return ".webp", "image/webp", nil
	default:
		return "", "", ErrImageTypeNotSupported
	}
}

func sanitizeFileSegment(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", "_", "-")
	value = replacer.Replace(value)
	if value == "" {
		return "card"
	}
	return value
}

func containsMarketplaceID(items []marketplaceOption, target string) bool {
	for _, item := range items {
		if item.ID == target {
			return true
		}
	}
	return false
}

func marketplacesToCatalog(items []marketplaceOption) []CatalogOption {
	result := make([]CatalogOption, len(items))
	for i, item := range items {
		result[i] = item.CatalogOption
	}
	return result
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

func containsInt(items []int, target int) bool {
	for _, item := range items {
		if item == target {
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

func cloneCatalogOptions(items []CatalogOption) []CatalogOption {
	return append([]CatalogOption(nil), items...)
}

func cloneCardTypeOptions(items []CardTypeOption) []CardTypeOption {
	return append([]CardTypeOption(nil), items...)
}

func cloneModelOptions(items []ModelOption) []ModelOption {
	return append([]ModelOption(nil), items...)
}

// buildCardPrompt составляет промпт для генерации одной карточки маркетплейса.
// Данные для промпта берутся напрямую из каталогов, поэтому добавление новых вариантов
// требует правок только в одном месте — в объявлении generateMarketplaces / generateStyles / generateCardTypes.
// Описание на английском — модели генерации изображений лучше понимают английские промпты.
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

// marketplaceAspectRatio возвращает рекомендуемое соотношение сторон карточки для маркетплейса.
// Значение берётся из каталога generateMarketplaces — менять нужно только там.
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
			return item.PromptPart
		}
	}
	return fallback
}

func findCatalogPromptPart(items []CatalogOption, id, fallback string) string {
	for _, item := range items {
		if item.ID == id {
			return item.PromptPart
		}
	}
	return fallback
}

func findCardTypePromptPart(items []CardTypeOption, id, fallback string) string {
	for _, item := range items {
		if item.ID == id {
			return item.PromptPart
		}
	}
	return fallback
}

// noopImageGenerator используется когда ROUTERAI_API_KEY не задан.
// Возвращает ошибку, чтобы генерация явно падала в failed, а не молча создавала пустые файлы.
type noopImageGenerator struct{}

func (noopImageGenerator) GenerateImage(_ context.Context, _ ImageGenerateInput) ([]byte, error) {
	return nil, fmt.Errorf("image generator is not configured (ROUTERAI_API_KEY is not set)")
}

func trimErrorMessage(err error) string {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "generation failed"
	}
	if len(message) > 500 {
		return message[:500]
	}
	return message
}
