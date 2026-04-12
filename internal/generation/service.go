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

const (
	defaultProjectTitle = "Новый проект"
)

var (
	generateMarketplaces = []CatalogOption{
		{ID: "wildberries", Label: "Wildberries"},
		{ID: "ozon", Label: "Ozon"},
		{ID: "yandex_market", Label: "Яндекс Маркет"},
	}
	generateStyles = []CatalogOption{
		{ID: "clean_catalog", Label: "Чистый каталог"},
		{ID: "accent_offer", Label: "Акцент на выгоде"},
		{ID: "premium_brand", Label: "Премиальный бренд"},
	}
	generateCardTypes = []CardTypeOption{
		{ID: "cover", Label: "Обложка", DefaultSelected: true},
		{ID: "benefits", Label: "Преимущества", DefaultSelected: true},
		{ID: "details", Label: "Детали", DefaultSelected: true},
		{ID: "usage", Label: "Сценарий использования"},
		{ID: "dimensions", Label: "Размеры"},
		{ID: "composition", Label: "Состав"},
	}
	generateCardCountOptions = []int{3, 5, 7, 10}
)

// CatalogOption описывает один доступный вариант из каталога generation-конфига.
type CatalogOption struct {
	ID    string
	Label string
}

// CardTypeOption описывает один тип карточки для страницы генерации.
type CardTypeOption struct {
	ID              string
	Label           string
	DefaultSelected bool
}

// Config описывает справочные данные для страницы `/app/generate`.
type Config struct {
	Marketplaces     []CatalogOption
	Styles           []CatalogOption
	CardTypes        []CardTypeOption
	CardCountOptions []int
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
	pool       *pgxpool.Pool
	queries    *dbgen.Queries
	jobsClient *jobs.Client
	storage    generationStorage
}

type generationStorage interface {
	Save(ctx context.Context, storageKey string, body []byte) (storage.SavedFile, error)
	Copy(ctx context.Context, sourceKey string, targetKey string) (storage.SavedFile, error)
	CreateZIP(ctx context.Context, targetKey string, files []storage.ArchiveFile) (storage.SavedFile, error)
	Delete(ctx context.Context, storageKey string) error
	PublicURL(storageKey string) string
}

// NewService создаёт сервис generation и связывает БД с локальным storage и очередью.
func NewService(pool *pgxpool.Pool, queries *dbgen.Queries, jobsClient *jobs.Client, storage generationStorage) *Service {
	return &Service{
		pool:       pool,
		queries:    queries,
		jobsClient: jobsClient,
		storage:    storage,
	}
}

// GetConfig возвращает каталог вариантов для страницы `/app/generate`.
func (s *Service) GetConfig(_ context.Context) Config {
	return Config{
		Marketplaces:     cloneCatalogOptions(generateMarketplaces),
		Styles:           cloneCatalogOptions(generateStyles),
		CardTypes:        cloneCardTypeOptions(generateCardTypes),
		CardCountOptions: append([]int(nil), generateCardCountOptions...),
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

	sourceAsset, err := s.queries.GetAssetByID(ctx, generationRow.SourceAssetID)
	if err != nil {
		return fmt.Errorf("get source asset for processing: %w", err)
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
	ext := extensionFromFilenameOrMime(sourceAsset.OriginalFilename, sourceAsset.MimeType)
	for i := 0; i < int(generationRow.CardCount); i++ {
		cardType := cardTypes[i%len(cardTypes)]
		targetKey := filepath.ToSlash(filepath.Join(
			"generated",
			generationID.String(),
			fmt.Sprintf("%02d-%s%s", i+1, sanitizeFileSegment(cardType.CardTypeID), ext),
		))

		savedFile, err := s.storage.Copy(ctx, sourceAsset.StorageKey, targetKey)
		if err != nil {
			return fmt.Errorf("copy generated card file: %w", err)
		}

		cardAssetID := uuid.New()
		if err := s.persistGeneratedCard(ctx, cardAssetID, generationRow, cardType.CardTypeID, int32(i), sourceAsset, savedFile); err != nil {
			_ = s.storage.Delete(ctx, savedFile.StorageKey)
			return err
		}

		archiveFiles = append(archiveFiles, storage.ArchiveFile{
			Name:       fmt.Sprintf("%02d-%s%s", i+1, sanitizeFileSegment(cardType.CardTypeID), ext),
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
	if !containsCatalogID(generateMarketplaces, marketplaceID) {
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
	}, nil
}

func (s *Service) persistGeneratedCard(
	ctx context.Context,
	assetID uuid.UUID,
	generationRow dbgen.Generation,
	cardTypeID string,
	position int32,
	sourceAsset dbgen.Asset,
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
		MimeType:         sourceAsset.MimeType,
		SizeBytes:        savedFile.SizeBytes,
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

func extensionFromFilenameOrMime(fileName string, mimeType string) string {
	ext, _, err := normalizeImageType(fileName, mimeType)
	if err == nil {
		return ext
	}
	return ".png"
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

func cloneCatalogOptions(items []CatalogOption) []CatalogOption {
	return append([]CatalogOption(nil), items...)
}

func cloneCardTypeOptions(items []CardTypeOption) []CardTypeOption {
	return append([]CardTypeOption(nil), items...)
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
