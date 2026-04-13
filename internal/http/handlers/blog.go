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
