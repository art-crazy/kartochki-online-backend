package blogsync

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sync записывает набор YAML-статей в PostgreSQL через idempotent upsert.
func Sync(ctx context.Context, pool *pgxpool.Pool, documents []Document) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin blog sync transaction: %w", err)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	for _, document := range documents {
		if err := syncDocument(ctx, tx, document); err != nil {
			return err
		}
	}

	if err := deleteStalePosts(ctx, tx, documents); err != nil {
		return err
	}
	if err := cleanupUnusedTaxonomy(ctx, tx, "blog_categories", "blog_post_categories", "blog_category_id"); err != nil {
		return err
	}
	if err := cleanupUnusedTaxonomy(ctx, tx, "blog_tags", "blog_post_tags", "blog_tag_id"); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit blog sync transaction: %w", err)
	}

	return nil
}

func syncDocument(ctx context.Context, tx pgx.Tx, document Document) error {
	postID, err := upsertPost(ctx, tx, document)
	if err != nil {
		return err
	}

	if err := upsertMetrics(ctx, tx, postID, document.Metrics); err != nil {
		return err
	}
	if err := replacePostTaxonomy(ctx, tx, postID, document.Categories, taxonomyConfig{
		taxonomyTable: "blog_categories",
		linkTable:     "blog_post_categories",
		linkColumn:    "blog_category_id",
		itemLabel:     "category",
	}); err != nil {
		return err
	}
	if err := replacePostTaxonomy(ctx, tx, postID, document.Tags, taxonomyConfig{
		taxonomyTable: "blog_tags",
		linkTable:     "blog_post_tags",
		linkColumn:    "blog_tag_id",
		itemLabel:     "tag",
	}); err != nil {
		return err
	}
	if err := replacePostSections(ctx, tx, postID, document.Sections); err != nil {
		return err
	}

	return nil
}

func upsertPost(ctx context.Context, tx pgx.Tx, document Document) (uuid.UUID, error) {
	var postID uuid.UUID
	err := tx.QueryRow(ctx, `
insert into blog_posts (slug, title, excerpt, seo_title, seo_description, status, published_at)
values ($1, $2, $3, $4, $5, $6, $7)
on conflict (slug) do update
set title = excluded.title,
    excerpt = excluded.excerpt,
    seo_title = excluded.seo_title,
    seo_description = excluded.seo_description,
    status = excluded.status,
    published_at = excluded.published_at,
    updated_at = now()
returning id
`, document.Slug, document.Title, document.Excerpt, nullableString(document.SEOTitle), nullableString(document.SEODescription), document.Status, document.PublishedAt).Scan(&postID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("upsert blog post %s: %w", document.Slug, err)
	}

	return postID, nil
}

func upsertMetrics(ctx context.Context, tx pgx.Tx, postID uuid.UUID, metrics Metrics) error {
	_, err := tx.Exec(ctx, `
insert into blog_post_metrics (blog_post_id, views_count, reading_time_minutes)
values ($1, $2, $3)
on conflict (blog_post_id) do update
set views_count = excluded.views_count,
    reading_time_minutes = excluded.reading_time_minutes,
    updated_at = now()
`, postID, metrics.ViewsCount, metrics.ReadingTimeMinutes)
	if err != nil {
		return fmt.Errorf("upsert blog metrics for %s: %w", postID, err)
	}

	return nil
}

type taxonomyConfig struct {
	taxonomyTable string
	linkTable     string
	linkColumn    string
	itemLabel     string
}

func replacePostTaxonomy(ctx context.Context, tx pgx.Tx, postID uuid.UUID, items []TaxonomyRef, cfg taxonomyConfig) error {
	deleteQuery := fmt.Sprintf(`delete from %s where blog_post_id = $1`, cfg.linkTable)
	if _, err := tx.Exec(ctx, deleteQuery, postID); err != nil {
		return fmt.Errorf("delete blog post %ss for %s: %w", cfg.itemLabel, postID, err)
	}

	insertQuery := fmt.Sprintf(`
insert into %s (blog_post_id, %s)
values ($1, $2)
on conflict (blog_post_id, %s) do nothing
`, cfg.linkTable, cfg.linkColumn, cfg.linkColumn)

	for _, item := range items {
		taxonomyID, err := upsertTaxonomy(ctx, tx, cfg.taxonomyTable, item)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, insertQuery, postID, taxonomyID); err != nil {
			return fmt.Errorf("attach %s %s to post %s: %w", cfg.itemLabel, item.Slug, postID, err)
		}
	}

	return nil
}

func replacePostSections(ctx context.Context, tx pgx.Tx, postID uuid.UUID, items []Section) error {
	if _, err := tx.Exec(ctx, `delete from blog_post_sections where blog_post_id = $1`, postID); err != nil {
		return fmt.Errorf("delete blog post sections for %s: %w", postID, err)
	}

	for index, item := range items {
		payload, err := json.Marshal(item.Payload)
		if err != nil {
			return fmt.Errorf("marshal section payload for post %s at index %d: %w", postID, index, err)
		}

		if _, err := tx.Exec(ctx, `
insert into blog_post_sections (blog_post_id, sort_order, kind, title, level, payload)
values ($1, $2, $3, $4, $5, $6)
`, postID, index, strings.TrimSpace(item.Kind), nullableString(item.Title), nullableInt(item.Level), payload); err != nil {
			return fmt.Errorf("insert blog section for post %s at index %d: %w", postID, index, err)
		}
	}

	return nil
}

func upsertTaxonomy(ctx context.Context, tx pgx.Tx, table string, item TaxonomyRef) (uuid.UUID, error) {
	var taxonomyID uuid.UUID
	query := fmt.Sprintf(`
insert into %s (slug, label)
values ($1, $2)
on conflict (slug) do update
set label = excluded.label
returning id
`, table)

	if err := tx.QueryRow(ctx, query, item.Slug, item.Label).Scan(&taxonomyID); err != nil {
		return uuid.Nil, fmt.Errorf("upsert %s %s: %w", table, item.Slug, err)
	}

	return taxonomyID, nil
}

func deleteStalePosts(ctx context.Context, tx pgx.Tx, documents []Document) error {
	slugs := make([]string, len(documents))
	for i, document := range documents {
		slugs[i] = document.Slug
	}

	if _, err := tx.Exec(ctx, `
delete from blog_posts
where not (slug = any($1::text[]))
`, slugs); err != nil {
		return fmt.Errorf("delete stale blog posts: %w", err)
	}

	return nil
}

func cleanupUnusedTaxonomy(ctx context.Context, tx pgx.Tx, taxonomyTable string, linkTable string, foreignKey string) error {
	query := fmt.Sprintf(`
delete from %s
where not exists (
	select 1
	from %s links
	where links.%s = %s.id
)
`, taxonomyTable, linkTable, foreignKey, taxonomyTable)

	if _, err := tx.Exec(ctx, query); err != nil {
		return fmt.Errorf("cleanup unused taxonomy in %s: %w", taxonomyTable, err)
	}

	return nil
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}
