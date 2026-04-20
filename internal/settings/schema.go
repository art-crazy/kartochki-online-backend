package settings

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func EnsureSchema(ctx context.Context, pool *pgxpool.Pool) error {
	const query = `
select exists (
    select 1
    from information_schema.columns
    where table_schema = 'public'
      and table_name = 'user_settings'
      and column_name = 'avatar_asset_id'
)`

	var ok bool
	if err := pool.QueryRow(ctx, query).Scan(&ok); err != nil {
		return fmt.Errorf("check settings schema: %w", err)
	}
	if !ok {
		return fmt.Errorf("missing settings schema migration: user_settings.avatar_asset_id")
	}

	return nil
}
