package settings

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kartochki-online-backend/internal/dbgen"
)

type UploadedAvatar struct {
	FileName    string
	ContentType string
	Body        []byte
}

type UploadedAvatarResult struct {
	AvatarURL string
}

// UploadAvatar сохраняет кастомный аватар пользователя и переключает профиль на него.
func (s *Service) UploadAvatar(ctx context.Context, userID string, image UploadedAvatar) (UploadedAvatarResult, error) {
	uid, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return UploadedAvatarResult{}, ErrUserNotFound
	}
	if s.storage == nil {
		return UploadedAvatarResult{}, fmt.Errorf("avatar storage is not configured")
	}

	image = normalizeUploadedAvatar(image)
	if len(image.Body) == 0 {
		return UploadedAvatarResult{}, ErrAvatarRequired
	}

	ext, mimeType, err := normalizeAvatarType(image.FileName, image.ContentType)
	if err != nil {
		return UploadedAvatarResult{}, err
	}

	assetID := uuid.New()
	storageKey := filepath.ToSlash(filepath.Join("avatars", uid.String(), assetID.String()+ext))
	savedFile, err := s.storage.Save(ctx, storageKey, image.Body)
	if err != nil {
		return UploadedAvatarResult{}, fmt.Errorf("save avatar file: %w", err)
	}
	cleanupSavedFile := func() {
		_ = s.storage.Delete(context.Background(), savedFile.StorageKey)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		cleanupSavedFile()
		return UploadedAvatarResult{}, fmt.Errorf("begin avatar upload tx: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)
	if _, err := txQueries.GetAuthUserByID(ctx, uid); err != nil {
		cleanupSavedFile()
		if errors.Is(err, pgx.ErrNoRows) {
			return UploadedAvatarResult{}, ErrUserNotFound
		}

		return UploadedAvatarResult{}, fmt.Errorf("get user before avatar upload: %w", err)
	}

	createdAsset, err := txQueries.CreateAsset(ctx, dbgen.CreateAssetParams{
		ID:               assetID,
		UserID:           uid,
		Kind:             "profile_avatar",
		StorageKey:       savedFile.StorageKey,
		OriginalFilename: image.FileName,
		MimeType:         mimeType,
		SizeBytes:        savedFile.SizeBytes,
	})
	if err != nil {
		cleanupSavedFile()
		return UploadedAvatarResult{}, fmt.Errorf("create avatar asset: %w", err)
	}

	oldAvatar, hasOldAvatar, err := getUserAvatarAsset(ctx, txQueries, uid)
	if err != nil {
		cleanupSavedFile()
		return UploadedAvatarResult{}, err
	}

	if err := txQueries.SetUserAvatarAssetID(ctx, dbgen.SetUserAvatarAssetIDParams{
		UserID:        uid,
		AvatarAssetID: pgtype.UUID{Bytes: [16]byte(assetID), Valid: true},
	}); err != nil {
		cleanupSavedFile()
		return UploadedAvatarResult{}, fmt.Errorf("set user avatar asset id: %w", err)
	}

	if hasOldAvatar {
		rows, err := txQueries.DeleteAssetByID(ctx, oldAvatar.ID)
		if err != nil {
			cleanupSavedFile()
			return UploadedAvatarResult{}, fmt.Errorf("delete previous avatar asset: %w", err)
		}
		if rows == 0 {
			cleanupSavedFile()
			return UploadedAvatarResult{}, fmt.Errorf("delete previous avatar asset: asset disappeared during upload")
		}
	}

	if err := tx.Commit(ctx); err != nil {
		cleanupSavedFile()
		return UploadedAvatarResult{}, fmt.Errorf("commit avatar upload tx: %w", err)
	}

	if hasOldAvatar {
		_ = s.storage.Delete(context.Background(), oldAvatar.StorageKey)
	}

	return UploadedAvatarResult{AvatarURL: s.storage.PublicURL(createdAsset.StorageKey)}, nil
}

func getUserAvatarAsset(ctx context.Context, queries *dbgen.Queries, userID uuid.UUID) (dbgen.Asset, bool, error) {
	asset, err := queries.GetUserAvatarAssetByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dbgen.Asset{}, false, nil
		}

		return dbgen.Asset{}, false, fmt.Errorf("get user avatar asset: %w", err)
	}

	return asset, true, nil
}

func normalizeUploadedAvatar(image UploadedAvatar) UploadedAvatar {
	image.FileName = strings.TrimSpace(image.FileName)
	image.ContentType = strings.TrimSpace(strings.ToLower(image.ContentType))
	return image
}

func normalizeAvatarType(fileName string, contentType string) (string, string, error) {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(fileName)))
	switch {
	case contentType == "image/png" || ext == ".png":
		return ".png", "image/png", nil
	case contentType == "image/jpeg" || contentType == "image/jpg" || ext == ".jpg" || ext == ".jpeg":
		return ".jpg", "image/jpeg", nil
	case contentType == "image/webp" || ext == ".webp":
		return ".webp", "image/webp", nil
	default:
		return "", "", ErrAvatarTypeNotSupported
	}
}
