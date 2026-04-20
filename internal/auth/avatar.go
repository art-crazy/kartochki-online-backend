package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kartochki-online-backend/internal/dbgen"
)

func (s *Service) customAvatarURL(ctx context.Context, queries *dbgen.Queries, userID uuid.UUID) string {
	if s.storage == nil {
		return ""
	}

	asset, err := queries.GetUserAvatarAssetByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ""
		}

		s.logger.Warn().Err(err).Str("user_id", userID.String()).Msg("не удалось загрузить пользовательский аватар")
		return ""
	}

	return s.storage.PublicURL(strings.TrimSpace(asset.StorageKey))
}
