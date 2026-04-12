package blog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kartochki-online-backend/internal/dbgen"
)

const (
	// DefaultPageSize задаёт размер страницы списка блога по умолчанию.
	DefaultPageSize = 12
	// MaxPageSize ограничивает размер страницы, чтобы публичный endpoint не становился слишком тяжёлым.
	MaxPageSize = 50
)

// ListInput описывает параметры серверной пагинации публичного списка статей.
type ListInput struct {
	Page     int
	PageSize int
}

// ListResult содержит данные для страницы `/blog`.
type ListResult struct {
	FeaturedPost *FeaturedPost
	Posts        []ListItem
	Categories   []Category
	PopularPosts []SidebarPost
	Tags         []Tag
	Pagination   Pagination
}

// FeaturedPost описывает главную статью над списком.
type FeaturedPost struct {
	Slug               string
	Title              string
	Excerpt            string
	PublishedAt        time.Time
	ReadingTimeMinutes int
}

// ListItem описывает карточку статьи в списке.
type ListItem struct {
	Slug               string
	Title              string
	Excerpt            string
	CategoryLabel      string
	PublishedAt        time.Time
	ReadingTimeMinutes int
}

// Category описывает категорию и количество статей в ней.
type Category struct {
	Slug  string
	Label string
	Count int
}

// SidebarPost описывает короткую ссылку на статью в боковом блоке.
type SidebarPost struct {
	Slug  string
	Title string
}

// Tag описывает публичный тег статьи.
type Tag struct {
	Slug  string
	Label string
}

// Pagination описывает параметры серверной пагинации списка.
type Pagination struct {
	Page       int
	PageSize   int
	TotalPages int
}

// Post содержит данные для страницы статьи `/blog/{slug}`.
type Post struct {
	Slug               string
	Title              string
	Description        string
	Excerpt            string
	PublishedAt        time.Time
	UpdatedAt          time.Time
	ReadingTimeMinutes int
	Views              int
	Tags               []Tag
	Sections           []ArticleSection
	RelatedPosts       []SidebarPost
}

// ArticleSection описывает один смысловой блок статьи.
type ArticleSection struct {
	ID      string
	Title   string
	Level   int
	Kind    SectionKind
	Body    string
	List    []string
	Table   *SectionTable
	Callout *SectionCallout
	Cards   []SectionCard
	Steps   []SectionStep
}

// SectionKind помогает фронтенду выбрать правильный рендеринг блока.
type SectionKind string

const (
	// SectionKindText означает обычный текстовый блок.
	SectionKindText SectionKind = "text"
	// SectionKindList означает блок со списком строк.
	SectionKindList SectionKind = "list"
	// SectionKindTable означает табличный блок.
	SectionKindTable SectionKind = "table"
	// SectionKindCallout означает заметку или предупреждение.
	SectionKindCallout SectionKind = "callout"
	// SectionKindCards означает сетку карточек.
	SectionKindCards SectionKind = "cards"
	// SectionKindSteps означает пошаговую инструкцию.
	SectionKindSteps SectionKind = "steps"
)

// SectionTable описывает таблицу внутри статьи.
type SectionTable struct {
	Head  []string
	Rows  [][]string
	Tones [][]string
}

// SectionCallout описывает выделенный информационный блок.
type SectionCallout struct {
	Tone  string
	Title string
	Text  string
}

// SectionCard описывает одну карточку в обзорной секции.
type SectionCard struct {
	Title string
	Tone  string
	Meta  []SectionCardMetaRow
}

// SectionCardMetaRow описывает строку "ключ-значение" внутри карточки.
type SectionCardMetaRow struct {
	Label string
	Value string
}

// SectionStep описывает один шаг внутри инструкции.
type SectionStep struct {
	Title       string
	Description string
}

// Service собирает публичные read-only ответы блога поверх sqlc-запросов.
type Service struct {
	queries *dbgen.Queries
}

// NewService создаёт сервис публичного блога.
func NewService(queries *dbgen.Queries) *Service {
	return &Service{queries: queries}
}

// NormalizeListInput заполняет значения по умолчанию и проверяет границы пагинации.
// При ошибке возвращает ErrInvalidPage или ErrInvalidPageSize — хендлер определяет поле по sentinel, а не по тексту.
func NormalizeListInput(input ListInput) (ListInput, error) {
	if input.Page == 0 {
		input.Page = 1
	}
	if input.PageSize == 0 {
		input.PageSize = DefaultPageSize
	}
	if input.Page < 1 {
		return ListInput{}, ErrInvalidPage
	}
	if input.PageSize < 1 || input.PageSize > MaxPageSize {
		return ListInput{}, ErrInvalidPageSize
	}
	return input, nil
}

// ListPublished собирает страницу публичного списка статей и связанные боковые блоки.
// Ожидает уже нормализованный input — вызывай NormalizeListInput перед передачей.
func (s *Service) ListPublished(ctx context.Context, input ListInput) (ListResult, error) {
	result := ListResult{
		Pagination: Pagination{
			Page:     input.Page,
			PageSize: input.PageSize,
		},
	}

	var featuredID uuid.UUID
	hasFeatured := false
	if input.Page == 1 {
		featuredRow, featuredErr := s.queries.GetFeaturedBlogPost(ctx)
		if featuredErr == nil {
			hasFeatured = true
			featuredID = featuredRow.ID
			result.FeaturedPost = &FeaturedPost{
				Slug:               strings.TrimSpace(featuredRow.Slug),
				Title:              strings.TrimSpace(featuredRow.Title),
				Excerpt:            strings.TrimSpace(featuredRow.Excerpt),
				PublishedAt:        featuredRow.PublishedAt.Time,
				ReadingTimeMinutes: int(featuredRow.ReadingTimeMinutes),
			}
		} else if !errors.Is(featuredErr, pgx.ErrNoRows) {
			return ListResult{}, fmt.Errorf("get featured blog post: %w", featuredErr)
		}
	}

	totalPages, err := s.listTotalPages(ctx, input, featuredID, hasFeatured)
	if err != nil {
		return ListResult{}, err
	}
	result.Pagination.TotalPages = totalPages

	posts, err := s.listPosts(ctx, input, featuredID, hasFeatured)
	if err != nil {
		return ListResult{}, err
	}
	result.Posts = posts

	categories, err := s.queries.ListBlogCategories(ctx)
	if err != nil {
		return ListResult{}, fmt.Errorf("list blog categories: %w", err)
	}
	result.Categories = toCategories(categories)

	popularPosts, err := s.queries.ListPopularBlogPosts(ctx)
	if err != nil {
		return ListResult{}, fmt.Errorf("list popular blog posts: %w", err)
	}
	result.PopularPosts = toPopularSidebarPosts(popularPosts)

	tags, err := s.queries.ListBlogTags(ctx)
	if err != nil {
		return ListResult{}, fmt.Errorf("list blog tags: %w", err)
	}
	result.Tags = toBlogTags(tags)

	return result, nil
}

// GetPublishedPostBySlug возвращает опубликованную статью, её секции и related-блок.
func (s *Service) GetPublishedPostBySlug(ctx context.Context, slug string) (Post, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return Post{}, ErrPostNotFound
	}

	postRow, err := s.queries.GetPublishedBlogPostBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Post{}, ErrPostNotFound
		}
		return Post{}, fmt.Errorf("get blog post by slug: %w", err)
	}

	tags, err := s.queries.ListBlogPostTags(ctx, postRow.ID)
	if err != nil {
		return Post{}, fmt.Errorf("list blog post tags: %w", err)
	}

	sectionsRows, err := s.queries.ListBlogPostSections(ctx, postRow.ID)
	if err != nil {
		return Post{}, fmt.Errorf("list blog post sections: %w", err)
	}

	sections, err := decodeSections(sectionsRows)
	if err != nil {
		return Post{}, err
	}

	relatedRows, err := s.queries.ListRelatedBlogPostsByPostID(ctx, postRow.ID)
	if err != nil {
		return Post{}, fmt.Errorf("list related blog posts: %w", err)
	}

	return Post{
		Slug:               strings.TrimSpace(postRow.Slug),
		Title:              strings.TrimSpace(postRow.Title),
		Description:        strings.TrimSpace(postRow.SeoDescription),
		Excerpt:            strings.TrimSpace(postRow.Excerpt),
		PublishedAt:        postRow.PublishedAt.Time,
		UpdatedAt:          postRow.UpdatedAt.Time,
		ReadingTimeMinutes: int(postRow.ReadingTimeMinutes),
		Views:              int(postRow.ViewsCount),
		Tags:               toPostTags(tags),
		Sections:           sections,
		RelatedPosts:       toRelatedSidebarPosts(relatedRows),
	}, nil
}

func (s *Service) listTotalPages(ctx context.Context, input ListInput, featuredID uuid.UUID, hasFeatured bool) (int, error) {
	var totalCount int64
	var err error

	if hasFeatured {
		totalCount, err = s.queries.CountPublishedBlogPostsExcludingPost(ctx, featuredID)
		if err != nil {
			return 0, fmt.Errorf("count published blog posts excluding featured: %w", err)
		}
	} else {
		totalCount, err = s.queries.CountPublishedBlogPosts(ctx)
		if err != nil {
			return 0, fmt.Errorf("count published blog posts: %w", err)
		}
	}

	if totalCount == 0 {
		return 0, nil
	}

	return int((totalCount + int64(input.PageSize) - 1) / int64(input.PageSize)), nil
}

func (s *Service) listPosts(ctx context.Context, input ListInput, featuredID uuid.UUID, hasFeatured bool) ([]ListItem, error) {
	offset := int32((input.Page - 1) * input.PageSize)
	limit := int32(input.PageSize)

	if hasFeatured {
		rows, err := s.queries.ListPublishedBlogPostsExcludingPost(ctx, dbgen.ListPublishedBlogPostsExcludingPostParams{
			BlogPostID: featuredID,
			LimitRows:  limit,
			OffsetRows: offset,
		})
		if err != nil {
			return nil, fmt.Errorf("list published blog posts excluding featured: %w", err)
		}
		result := make([]ListItem, len(rows))
		for i, row := range rows {
			result[i] = ListItem{
				Slug:               strings.TrimSpace(row.Slug),
				Title:              strings.TrimSpace(row.Title),
				Excerpt:            strings.TrimSpace(row.Excerpt),
				CategoryLabel:      strings.TrimSpace(row.CategoryLabel),
				PublishedAt:        row.PublishedAt.Time,
				ReadingTimeMinutes: int(row.ReadingTimeMinutes),
			}
		}
		return result, nil
	}

	rows, err := s.queries.ListPublishedBlogPosts(ctx, dbgen.ListPublishedBlogPostsParams{
		LimitRows:  limit,
		OffsetRows: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list published blog posts: %w", err)
	}
	result := make([]ListItem, len(rows))
	for i, row := range rows {
		result[i] = ListItem{
			Slug:               strings.TrimSpace(row.Slug),
			Title:              strings.TrimSpace(row.Title),
			Excerpt:            strings.TrimSpace(row.Excerpt),
			CategoryLabel:      strings.TrimSpace(row.CategoryLabel),
			PublishedAt:        row.PublishedAt.Time,
			ReadingTimeMinutes: int(row.ReadingTimeMinutes),
		}
	}
	return result, nil
}

type sectionTextPayload struct {
	Body string `json:"body"`
}

type sectionListPayload struct {
	Items []string `json:"items"`
}

type sectionTablePayload struct {
	Head  []string   `json:"head"`
	Rows  [][]string `json:"rows"`
	Tones [][]string `json:"tones"`
}

type sectionCalloutPayload struct {
	Tone  string `json:"tone"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

type sectionCardsPayload struct {
	Cards []SectionCard `json:"cards"`
}

type sectionStepsPayload struct {
	Steps []SectionStep `json:"steps"`
}

func decodeSections(rows []dbgen.ListBlogPostSectionsRow) ([]ArticleSection, error) {
	result := make([]ArticleSection, len(rows))
	for i, row := range rows {
		section := ArticleSection{
			ID:    row.ID.String(),
			Title: strings.TrimSpace(row.Title),
			Level: int(row.Level),
			Kind:  SectionKind(strings.TrimSpace(row.Kind)),
		}

		switch section.Kind {
		case SectionKindText:
			var payload sectionTextPayload
			if err := json.Unmarshal(row.Payload, &payload); err != nil {
				return nil, fmt.Errorf("decode text section payload %s: %w", row.ID, err)
			}
			section.Body = strings.TrimSpace(payload.Body)
		case SectionKindList:
			var payload sectionListPayload
			if err := json.Unmarshal(row.Payload, &payload); err != nil {
				return nil, fmt.Errorf("decode list section payload %s: %w", row.ID, err)
			}
			section.List = trimStrings(payload.Items)
		case SectionKindTable:
			var payload sectionTablePayload
			if err := json.Unmarshal(row.Payload, &payload); err != nil {
				return nil, fmt.Errorf("decode table section payload %s: %w", row.ID, err)
			}
			section.Table = &SectionTable{
				Head:  trimStrings(payload.Head),
				Rows:  trimMatrix(payload.Rows),
				Tones: trimMatrix(payload.Tones),
			}
		case SectionKindCallout:
			var payload sectionCalloutPayload
			if err := json.Unmarshal(row.Payload, &payload); err != nil {
				return nil, fmt.Errorf("decode callout section payload %s: %w", row.ID, err)
			}
			section.Callout = &SectionCallout{
				Tone:  strings.TrimSpace(payload.Tone),
				Title: strings.TrimSpace(payload.Title),
				Text:  strings.TrimSpace(payload.Text),
			}
		case SectionKindCards:
			var payload sectionCardsPayload
			if err := json.Unmarshal(row.Payload, &payload); err != nil {
				return nil, fmt.Errorf("decode cards section payload %s: %w", row.ID, err)
			}
			section.Cards = trimCards(payload.Cards)
		case SectionKindSteps:
			var payload sectionStepsPayload
			if err := json.Unmarshal(row.Payload, &payload); err != nil {
				return nil, fmt.Errorf("decode steps section payload %s: %w", row.ID, err)
			}
			section.Steps = trimSteps(payload.Steps)
		default:
			return nil, fmt.Errorf("unsupported blog section kind %q", row.Kind)
		}

		result[i] = section
	}

	return result, nil
}

func toCategories(rows []dbgen.ListBlogCategoriesRow) []Category {
	result := make([]Category, len(rows))
	for i, row := range rows {
		result[i] = Category{
			Slug:  strings.TrimSpace(row.Slug),
			Label: strings.TrimSpace(row.Label),
			Count: int(row.PostsCount),
		}
	}
	return result
}

func toPopularSidebarPosts(rows []dbgen.ListPopularBlogPostsRow) []SidebarPost {
	result := make([]SidebarPost, len(rows))
	for i, row := range rows {
		result[i] = SidebarPost{
			Slug:  strings.TrimSpace(row.Slug),
			Title: strings.TrimSpace(row.Title),
		}
	}
	return result
}

func toRelatedSidebarPosts(rows []dbgen.ListRelatedBlogPostsByPostIDRow) []SidebarPost {
	result := make([]SidebarPost, len(rows))
	for i, row := range rows {
		result[i] = SidebarPost{
			Slug:  strings.TrimSpace(row.Slug),
			Title: strings.TrimSpace(row.Title),
		}
	}
	return result
}

func toBlogTags(rows []dbgen.ListBlogTagsRow) []Tag {
	result := make([]Tag, len(rows))
	for i, row := range rows {
		result[i] = Tag{
			Slug:  strings.TrimSpace(row.Slug),
			Label: strings.TrimSpace(row.Label),
		}
	}
	return result
}

func toPostTags(rows []dbgen.ListBlogPostTagsRow) []Tag {
	result := make([]Tag, len(rows))
	for i, row := range rows {
		result[i] = Tag{
			Slug:  strings.TrimSpace(row.Slug),
			Label: strings.TrimSpace(row.Label),
		}
	}
	return result
}


func trimStrings(items []string) []string {
	result := make([]string, len(items))
	for i, item := range items {
		result[i] = strings.TrimSpace(item)
	}
	return result
}

func trimMatrix(items [][]string) [][]string {
	result := make([][]string, len(items))
	for i, row := range items {
		result[i] = trimStrings(row)
	}
	return result
}

func trimCards(items []SectionCard) []SectionCard {
	result := make([]SectionCard, len(items))
	for i, item := range items {
		result[i] = SectionCard{
			Title: strings.TrimSpace(item.Title),
			Tone:  strings.TrimSpace(item.Tone),
			Meta:  trimCardMeta(item.Meta),
		}
	}
	return result
}

func trimCardMeta(items []SectionCardMetaRow) []SectionCardMetaRow {
	result := make([]SectionCardMetaRow, len(items))
	for i, item := range items {
		result[i] = SectionCardMetaRow{
			Label: strings.TrimSpace(item.Label),
			Value: strings.TrimSpace(item.Value),
		}
	}
	return result
}

func trimSteps(items []SectionStep) []SectionStep {
	result := make([]SectionStep, len(items))
	for i, item := range items {
		result[i] = SectionStep{
			Title:       strings.TrimSpace(item.Title),
			Description: strings.TrimSpace(item.Description),
		}
	}
	return result
}
