package generation

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kartochki-online-backend/internal/dbgen"
	"kartochki-online-backend/internal/platform/storage"
)

func (s *Service) persistGeneratedCard(
	ctx context.Context,
	assetID uuid.UUID,
	generationRow dbgen.Generation,
	cardTypeID string,
	position int32,
	savedFile storage.SavedFile,
) error {
	// Asset и связь с generation создаются одной транзакцией, чтобы polling не увидел неполную карточку.
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

// persistArchiveAsset сохраняет DB-запись архива после того, как ZIP уже создан в storage.
// Ссылка на архив появится в generation отдельно, когда вся обработка успешно завершится.
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
