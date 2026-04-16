package generation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"kartochki-online-backend/internal/dbgen"
	"kartochki-online-backend/internal/platform/storage"
)

// Service управляет generation-сценарием поверх sqlc, storage и очереди фоновых задач.
type Service struct {
	pool           *pgxpool.Pool
	queries        *dbgen.Queries
	jobEnqueuer    GenerationJobEnqueuer
	storage        generationStorage
	limits         generationLimits
	imageGenerator ImageGenerator
}

type generationStorage interface {
	Save(ctx context.Context, storageKey string, body []byte) (storage.SavedFile, error)
	Read(ctx context.Context, storageKey string) ([]byte, error)
	CreateZIP(ctx context.Context, targetKey string, files []storage.ArchiveFile) (storage.SavedFile, error)
	Delete(ctx context.Context, storageKey string) error
	PublicURL(storageKey string) string
}

type generationLimits interface {
	EnsureGenerationAllowed(ctx context.Context, userID string, requestedCards int) error
}

// GenerationJobEnqueuer ставит generation-задачу в фоновую очередь.
// Интерфейс скрывает конкретный брокер задач от generation-сервиса.
type GenerationJobEnqueuer interface {
	EnqueueGeneration(ctx context.Context, generationID string) error
}

// NewService создаёт сервис generation и связывает БД с локальным storage и очередью.
// imageGenerator может быть nil. Тогда используется noopImageGenerator, который возвращает ошибку.
func NewService(pool *pgxpool.Pool, queries *dbgen.Queries, jobEnqueuer GenerationJobEnqueuer, storage generationStorage, limits generationLimits, imageGenerator ImageGenerator) *Service {
	gen := imageGenerator
	if gen == nil {
		gen = noopImageGenerator{}
	}
	return &Service{
		pool:           pool,
		queries:        queries,
		jobEnqueuer:    jobEnqueuer,
		storage:        storage,
		limits:         limits,
		imageGenerator: gen,
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
		_ = s.storage.Delete(ctx, saved.StorageKey)
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

	if s.jobEnqueuer == nil {
		_ = s.queries.MarkGenerationFailed(ctx, dbgen.MarkGenerationFailedParams{
			ID:           generationID,
			ErrorMessage: "background queue is not configured",
		})
		return CreatedGeneration{}, fmt.Errorf("generation queue is not configured")
	}

	if err := s.jobEnqueuer.EnqueueGeneration(ctx, generationID.String()); err != nil {
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
