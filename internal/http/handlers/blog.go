package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"kartochki-online-backend/internal/blog"
	"kartochki-online-backend/internal/http/contracts"
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

func parseBlogListInput(r *http.Request) (blog.ListInput, []contracts.ErrorDetail) {
	query := r.URL.Query()
	input := blog.ListInput{}
	var details []contracts.ErrorDetail

	// Значение 0 означает «параметр не передан» — NormalizeListInput подставит дефолты.
	// Некорректный диапазон (< 1 или > MaxPageSize) вернёт ошибку уже в нормализации.
	if rawPage := strings.TrimSpace(query.Get("page")); rawPage != "" {
		page, err := strconv.Atoi(rawPage)
		if err != nil {
			details = append(details, contracts.ErrorDetail{Field: "page", Message: "must be an integer"})
		} else {
			input.Page = page
		}
	}

	if rawPageSize := strings.TrimSpace(query.Get("page_size")); rawPageSize != "" {
		pageSize, err := strconv.Atoi(rawPageSize)
		if err != nil {
			details = append(details, contracts.ErrorDetail{Field: "page_size", Message: "must be an integer"})
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
			details = append(details, contracts.ErrorDetail{Field: "page_size", Message: err.Error()})
		default:
			details = append(details, contracts.ErrorDetail{Field: "page", Message: err.Error()})
		}
		return blog.ListInput{}, details
	}

	return normalized, nil
}

func toBlogListResponse(result blog.ListResult) contracts.BlogListResponse {
	responsePayload := contracts.BlogListResponse{
		Posts:        toBlogListItems(result.Posts),
		Categories:   toBlogCategories(result.Categories),
		PopularPosts: toBlogSidebarPosts(result.PopularPosts),
		Tags:         toBlogTags(result.Tags),
		Pagination: contracts.Pagination{
			Page:       result.Pagination.Page,
			PageSize:   result.Pagination.PageSize,
			TotalPages: result.Pagination.TotalPages,
		},
	}

	if result.FeaturedPost != nil {
		responsePayload.FeaturedPost = &contracts.FeaturedBlogPost{
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

func toBlogPostResponse(post blog.Post) contracts.BlogPostResponse {
	return contracts.BlogPostResponse{
		Post: contracts.BlogPostDetail{
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

func toBlogListItems(items []blog.ListItem) []contracts.BlogListItem {
	result := make([]contracts.BlogListItem, len(items))
	for i, item := range items {
		result[i] = contracts.BlogListItem{
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

func toBlogCategories(items []blog.Category) []contracts.BlogCategory {
	result := make([]contracts.BlogCategory, len(items))
	for i, item := range items {
		result[i] = contracts.BlogCategory{
			Slug:  item.Slug,
			Label: item.Label,
			Count: item.Count,
		}
	}
	return result
}

func toBlogSidebarPosts(items []blog.SidebarPost) []contracts.BlogSidebarPost {
	result := make([]contracts.BlogSidebarPost, len(items))
	for i, item := range items {
		result[i] = contracts.BlogSidebarPost{
			Slug:          item.Slug,
			Title:         item.Title,
			CanonicalPath: blogCanonicalPrefix + item.Slug,
		}
	}
	return result
}

func toRelatedBlogPosts(items []blog.SidebarPost) []contracts.RelatedBlogPost {
	result := make([]contracts.RelatedBlogPost, len(items))
	for i, item := range items {
		result[i] = contracts.RelatedBlogPost{
			Slug:          item.Slug,
			Title:         item.Title,
			CanonicalPath: blogCanonicalPrefix + item.Slug,
		}
	}
	return result
}

func toBlogTags(items []blog.Tag) []contracts.BlogTag {
	result := make([]contracts.BlogTag, len(items))
	for i, item := range items {
		result[i] = contracts.BlogTag{
			Slug:  item.Slug,
			Label: item.Label,
		}
	}
	return result
}

func toBlogArticleSections(items []blog.ArticleSection) []contracts.BlogArticleSection {
	result := make([]contracts.BlogArticleSection, len(items))
	for i, item := range items {
		result[i] = contracts.BlogArticleSection{
			ID:      item.ID,
			Title:   item.Title,
			Level:   item.Level,
			Kind:    contracts.BlogSectionKind(item.Kind),
			Body:    item.Body,
			List:    item.List,
			Table:   toBlogSectionTable(item.Table),
			Callout: toBlogSectionCallout(item.Callout),
			Cards:   toBlogSectionCards(item.Cards),
			Steps:   toBlogSectionSteps(item.Steps),
		}
	}
	return result
}

func toBlogSectionTable(table *blog.SectionTable) *contracts.BlogSectionTable {
	if table == nil {
		return nil
	}
	return &contracts.BlogSectionTable{
		Head:  table.Head,
		Rows:  table.Rows,
		Tones: table.Tones,
	}
}

func toBlogSectionCallout(callout *blog.SectionCallout) *contracts.BlogSectionCallout {
	if callout == nil {
		return nil
	}
	return &contracts.BlogSectionCallout{
		Tone:  callout.Tone,
		Title: callout.Title,
		Text:  callout.Text,
	}
}

func toBlogSectionCards(items []blog.SectionCard) []contracts.BlogSectionCard {
	result := make([]contracts.BlogSectionCard, len(items))
	for i, item := range items {
		result[i] = contracts.BlogSectionCard{
			Title: item.Title,
			Tone:  item.Tone,
			Meta:  toBlogSectionCardMetaRows(item.Meta),
		}
	}
	return result
}

func toBlogSectionCardMetaRows(items []blog.SectionCardMetaRow) []contracts.BlogSectionCardMetaRow {
	result := make([]contracts.BlogSectionCardMetaRow, len(items))
	for i, item := range items {
		result[i] = contracts.BlogSectionCardMetaRow{
			Label: item.Label,
			Value: item.Value,
		}
	}
	return result
}

func toBlogSectionSteps(items []blog.SectionStep) []contracts.BlogSectionStep {
	result := make([]contracts.BlogSectionStep, len(items))
	for i, item := range items {
		result[i] = contracts.BlogSectionStep{
			Title:       item.Title,
			Description: item.Description,
		}
	}
	return result
}
