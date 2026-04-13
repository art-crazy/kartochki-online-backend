package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	openapi "kartochki-online-backend/api/gen"
	"kartochki-online-backend/internal/blog"
	"kartochki-online-backend/internal/http/requestctx"
	"kartochki-online-backend/internal/http/response"
)

const blogCanonicalPrefix = "/blog/"

// publicBlogService описывает read-only сценарии публичного блога.
type publicBlogService interface {
	ListPublished(ctx context.Context, input blog.ListInput) (blog.ListResult, error)
	GetPublishedPostBySlug(ctx context.Context, slug string) (blog.Post, error)
}

// BlogHandler обслуживает публичные SEO-маршруты блога.
type BlogHandler struct {
	service publicBlogService
	logger  zerolog.Logger
}

// NewBlogHandler создаёт обработчик публичного blog API.
func NewBlogHandler(service publicBlogService, logger zerolog.Logger) BlogHandler {
	return BlogHandler{
		service: service,
		logger:  logger,
	}
}

// List возвращает страницу списка статей `/api/v1/public/blog`.
func (h BlogHandler) List(w http.ResponseWriter, r *http.Request) {
	input, details := parseBlogListInput(r)
	if len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", details...)
		return
	}

	result, err := h.service.ListPublished(r.Context(), input)
	if err != nil {
		logger := requestctx.Logger(r.Context(), h.logger)
		logger.Error().Err(err).Msg("не удалось загрузить публичный список статей блога")
		response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to load blog")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, toBlogListResponse(result))
}

// GetBySlug возвращает одну опубликованную статью `/api/v1/public/blog/{slug}`.
func (h BlogHandler) GetBySlug(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(chi.URLParam(r, "slug"))
	if slug == "" {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "slug is required")
		return
	}

	post, err := h.service.GetPublishedPostBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, blog.ErrPostNotFound) {
			response.WriteError(w, r, http.StatusNotFound, "blog_post_not_found", "blog post not found")
			return
		}

		logger := requestctx.Logger(r.Context(), h.logger)
		logger.Error().Err(err).Str("slug", slug).Msg("не удалось загрузить статью блога")
		response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to load blog post")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, toBlogPostResponse(post))
}

func parseBlogListInput(r *http.Request) (blog.ListInput, []openapi.ErrorDetail) {
	query := r.URL.Query()
	input := blog.ListInput{}
	var details []openapi.ErrorDetail

	// Значение 0 означает «параметр не передан» — NormalizeListInput подставит дефолты.
	// Некорректный диапазон (< 1 или > MaxPageSize) вернёт ошибку уже в нормализации.
	if rawPage := strings.TrimSpace(query.Get("page")); rawPage != "" {
		page, err := strconv.Atoi(rawPage)
		if err != nil {
			details = append(details, openapi.ErrorDetail{Field: strPtr("page"), Message: "must be an integer"})
		} else {
			input.Page = page
		}
	}

	if rawPageSize := strings.TrimSpace(query.Get("page_size")); rawPageSize != "" {
		pageSize, err := strconv.Atoi(rawPageSize)
		if err != nil {
			details = append(details, openapi.ErrorDetail{Field: strPtr("page_size"), Message: "must be an integer"})
		} else {
			input.PageSize = pageSize
		}
	}

	if len(details) > 0 {
		return blog.ListInput{}, details
	}

	normalized, err := blog.NormalizeListInput(input)
	if err != nil {
		switch {
		case errors.Is(err, blog.ErrInvalidPageSize):
			details = append(details, openapi.ErrorDetail{Field: strPtr("page_size"), Message: err.Error()})
		default:
			details = append(details, openapi.ErrorDetail{Field: strPtr("page"), Message: err.Error()})
		}
		return blog.ListInput{}, details
	}

	return normalized, nil
}

func toBlogListResponse(result blog.ListResult) openapi.BlogListResponse {
	responsePayload := openapi.BlogListResponse{
		Posts:        toBlogListItems(result.Posts),
		Categories:   toBlogCategories(result.Categories),
		PopularPosts: toBlogSidebarPosts(result.PopularPosts),
		Tags:         toBlogTags(result.Tags),
		Pagination: openapi.BlogPagination{
			Page:       result.Pagination.Page,
			PageSize:   result.Pagination.PageSize,
			TotalPages: result.Pagination.TotalPages,
		},
	}

	if result.FeaturedPost != nil {
		responsePayload.FeaturedPost = &openapi.FeaturedBlogPost{
			Slug:               result.FeaturedPost.Slug,
			Title:              result.FeaturedPost.Title,
			Excerpt:            result.FeaturedPost.Excerpt,
			PublishedAt:        result.FeaturedPost.PublishedAt,
			ReadingTimeMinutes: result.FeaturedPost.ReadingTimeMinutes,
			CanonicalPath:      blogCanonicalPrefix + result.FeaturedPost.Slug,
		}
	}

	return responsePayload
}

func toBlogPostResponse(post blog.Post) openapi.BlogPostResponse {
	return openapi.BlogPostResponse{
		Post: openapi.BlogPostDetail{
			Slug:               post.Slug,
			Title:              post.Title,
			Description:        post.Description,
			Excerpt:            post.Excerpt,
			CanonicalPath:      blogCanonicalPrefix + post.Slug,
			PublishedAt:        post.PublishedAt,
			UpdatedAt:          post.UpdatedAt,
			ReadingTimeMinutes: post.ReadingTimeMinutes,
			Views:              post.Views,
			Tags:               toBlogTags(post.Tags),
		},
		ArticleSections: toBlogArticleSections(post.Sections),
		RelatedPosts:    toRelatedBlogPosts(post.RelatedPosts),
	}
}

func toBlogListItems(items []blog.ListItem) []openapi.BlogListItem {
	result := make([]openapi.BlogListItem, len(items))
	for i, item := range items {
		result[i] = openapi.BlogListItem{
			Slug:               item.Slug,
			Title:              item.Title,
			Excerpt:            item.Excerpt,
			CategoryLabel:      item.CategoryLabel,
			PublishedAt:        item.PublishedAt,
			ReadingTimeMinutes: item.ReadingTimeMinutes,
			CanonicalPath:      blogCanonicalPrefix + item.Slug,
		}
	}
	return result
}

func toBlogCategories(items []blog.Category) []openapi.BlogCategory {
	result := make([]openapi.BlogCategory, len(items))
	for i, item := range items {
		result[i] = openapi.BlogCategory{
			Slug:  item.Slug,
			Label: item.Label,
			Count: item.Count,
		}
	}
	return result
}

func toBlogSidebarPosts(items []blog.SidebarPost) []openapi.BlogSidebarPost {
	result := make([]openapi.BlogSidebarPost, len(items))
	for i, item := range items {
		result[i] = openapi.BlogSidebarPost{
			Slug:          item.Slug,
			Title:         item.Title,
			CanonicalPath: blogCanonicalPrefix + item.Slug,
		}
	}
	return result
}

func toRelatedBlogPosts(items []blog.SidebarPost) []openapi.RelatedBlogPost {
	result := make([]openapi.RelatedBlogPost, len(items))
	for i, item := range items {
		result[i] = openapi.RelatedBlogPost{
			Slug:          item.Slug,
			Title:         item.Title,
			CanonicalPath: blogCanonicalPrefix + item.Slug,
		}
	}
	return result
}

func toBlogTags(items []blog.Tag) []openapi.BlogTag {
	result := make([]openapi.BlogTag, len(items))
	for i, item := range items {
		result[i] = openapi.BlogTag{
			Slug:  item.Slug,
			Label: item.Label,
		}
	}
	return result
}

func toBlogArticleSections(items []blog.ArticleSection) []openapi.BlogArticleSection {
	result := make([]openapi.BlogArticleSection, len(items))
	for i, item := range items {
		id := mustParseUUID(item.ID)

		section := openapi.BlogArticleSection{
			Id:      id,
			Title:   item.Title,
			Level:   item.Level,
			Kind:    openapi.BlogSectionKind(item.Kind),
			Table:   toBlogSectionTable(item.Table),
			Callout: toBlogSectionCallout(item.Callout),
		}
		if item.Body != "" {
			section.Body = &item.Body
		}
		if len(item.List) > 0 {
			list := item.List
			section.List = &list
		}
		if len(item.Cards) > 0 {
			cards := toBlogSectionCards(item.Cards)
			section.Cards = &cards
		}
		if len(item.Steps) > 0 {
			steps := toBlogSectionSteps(item.Steps)
			section.Steps = &steps
		}
		result[i] = section
	}
	return result
}

func toBlogSectionTable(table *blog.SectionTable) *openapi.BlogSectionTable {
	if table == nil {
		return nil
	}
	t := &openapi.BlogSectionTable{
		Head: table.Head,
		Rows: table.Rows,
	}
	if len(table.Tones) > 0 {
		t.Tones = &table.Tones
	}
	return t
}

func toBlogSectionCallout(callout *blog.SectionCallout) *openapi.BlogSectionCallout {
	if callout == nil {
		return nil
	}
	return &openapi.BlogSectionCallout{
		Tone:  callout.Tone,
		Title: callout.Title,
		Text:  callout.Text,
	}
}

func toBlogSectionCards(items []blog.SectionCard) []openapi.BlogSectionCard {
	result := make([]openapi.BlogSectionCard, len(items))
	for i, item := range items {
		card := openapi.BlogSectionCard{
			Title: item.Title,
		}
		if item.Tone != "" {
			card.Tone = &item.Tone
		}
		if len(item.Meta) > 0 {
			meta := toBlogSectionCardMetaRows(item.Meta)
			card.Meta = &meta
		}
		result[i] = card
	}
	return result
}

func toBlogSectionCardMetaRows(items []blog.SectionCardMetaRow) []openapi.BlogSectionCardMetaRow {
	result := make([]openapi.BlogSectionCardMetaRow, len(items))
	for i, item := range items {
		result[i] = openapi.BlogSectionCardMetaRow{
			Label: item.Label,
			Value: item.Value,
		}
	}
	return result
}

func toBlogSectionSteps(items []blog.SectionStep) []openapi.BlogSectionStep {
	result := make([]openapi.BlogSectionStep, len(items))
	for i, item := range items {
		result[i] = openapi.BlogSectionStep{
			Title:       item.Title,
			Description: item.Description,
		}
	}
	return result
}
