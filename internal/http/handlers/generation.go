package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	openapi "kartochki-online-backend/api/gen"
	"kartochki-online-backend/internal/generation"
	"kartochki-online-backend/internal/http/requestctx"
	"kartochki-online-backend/internal/http/response"
)

const maxUploadImageSizeBytes = 15 << 20

// generationService описывает сценарии generation, которые доступны HTTP-слою.
type generationService interface {
	GetConfig(ctx context.Context) generation.Config
	UploadSourceImage(ctx context.Context, userID string, image generation.UploadedImage) (generation.UploadedAsset, error)
	Create(ctx context.Context, input generation.CreateInput) (generation.CreatedGeneration, error)
	GetByID(ctx context.Context, userID string, generationID string) (generation.Status, error)
}

// GenerationHandler обслуживает endpoints страницы `/app/generate`.
type GenerationHandler struct {
	service generationService
	logger  zerolog.Logger
}

// NewGenerationHandler создаёт handler generation API.
func NewGenerationHandler(service generationService, logger zerolog.Logger) GenerationHandler {
	return GenerationHandler{
		service: service,
		logger:  logger,
	}
}

// GetConfig возвращает справочные данные для формы запуска генерации.
func (h GenerationHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := currentUserFromCtx(w, r); !ok {
		return
	}

	cfg := h.service.GetConfig(r.Context())

	response.WriteJSON(w, r, http.StatusOK, openapi.GenerateConfigResponse{
		Marketplaces:     toGenerateMarketplaces(cfg.Marketplaces),
		Styles:           toGenerateStyles(cfg.Styles),
		CardTypes:        toGenerateCardTypes(cfg.CardTypes),
		CardCountOptions: cfg.CardCountOptions,
	})
}

// UploadImage принимает multipart upload исходного изображения и сохраняет его как source asset.
func (h GenerationHandler) UploadImage(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUserFromCtx(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadImageSizeBytes)
	if err := r.ParseMultipartForm(maxUploadImageSizeBytes); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_multipart", "request must contain one image file")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "file_required", "multipart field file is required")
		return
	}
	defer file.Close()

	body, err := io.ReadAll(file)
	if err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_upload", "failed to read uploaded image")
		return
	}

	uploadedAsset, err := h.service.UploadSourceImage(r.Context(), user.ID, generation.UploadedImage{
		FileName:    header.Filename,
		ContentType: header.Header.Get("Content-Type"),
		Body:        body,
	})
	if err != nil {
		switch {
		case errors.Is(err, generation.ErrImageRequired):
			response.WriteError(w, r, http.StatusBadRequest, "file_required", "multipart field file is required")
		case errors.Is(err, generation.ErrImageTypeNotSupported):
			response.WriteError(w, r, http.StatusBadRequest, "unsupported_image_type", "only png, jpg and webp images are supported")
		default:
			logger := h.requestLogger(r)
			logger.Error().Err(err).Str("user_id", user.ID).Msg("не удалось сохранить исходное изображение")
			response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to upload image")
		}
		return
	}

	response.WriteJSON(w, r, http.StatusCreated, openapi.UploadImageResponse{
		AssetId:    mustParseUUID(uploadedAsset.AssetID),
		PreviewUrl: uploadedAsset.PreviewURL,
	})
}

// CreateGeneration создаёт проект и ставит запуск генерации в очередь.
func (h GenerationHandler) CreateGeneration(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUserFromCtx(w, r)
	if !ok {
		return
	}

	var req openapi.CreateGenerationRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if details := validateCreateGenerationRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", details...)
		return
	}

	result, err := h.service.Create(r.Context(), generation.CreateInput{
		UserID:        user.ID,
		ProjectName:   stringOrEmpty(req.ProjectName),
		MarketplaceID: req.MarketplaceId,
		StyleID:       req.StyleId,
		CardTypeIDs:   req.CardTypeIds,
		CardCount:     req.CardCount,
		SourceAssetID: req.SourceAssetId.String(),
	})
	if err != nil {
		switch {
		case errors.Is(err, generation.ErrSourceAssetNotFound):
			response.WriteError(w, r, http.StatusNotFound, "source_asset_not_found", "source image not found")
		case errors.Is(err, generation.ErrInvalidMarketplace):
			response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", openapi.ErrorDetail{
				Field:   strPtr("marketplace_id"),
				Message: "unknown marketplace",
			})
		case errors.Is(err, generation.ErrInvalidStyle):
			response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", openapi.ErrorDetail{
				Field:   strPtr("style_id"),
				Message: "unknown style",
			})
		case errors.Is(err, generation.ErrInvalidCardType):
			response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", openapi.ErrorDetail{
				Field:   strPtr("card_type_ids"),
				Message: "one or more card types are invalid",
			})
		case errors.Is(err, generation.ErrInvalidCardCount):
			response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", openapi.ErrorDetail{
				Field:   strPtr("card_count"),
				Message: "unsupported card count",
			})
		case errors.Is(err, generation.ErrProjectNameTooLong):
			response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", openapi.ErrorDetail{
				Field:   strPtr("project_name"),
				Message: "must be at most 200 characters",
			})
		case errors.Is(err, generation.ErrQuotaExceeded):
			response.WriteError(w, r, http.StatusConflict, "generation_quota_exceeded", "generation quota is exceeded")
		default:
			logger := h.requestLogger(r)
			logger.Error().Err(err).Str("user_id", user.ID).Msg("не удалось запустить генерацию")
			response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to create generation")
		}
		return
	}

	response.WriteJSON(w, r, http.StatusAccepted, openapi.CreateGenerationResponse{
		GenerationId: mustParseUUID(result.GenerationID),
		Status:       openapi.CreateGenerationResponseStatusQueued,
	})
}

// GetGenerationStatus возвращает текущий статус generation job и итоговые карточки после завершения.
func (h GenerationHandler) GetGenerationStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUserFromCtx(w, r)
	if !ok {
		return
	}

	generationID := chi.URLParam(r, "id")
	result, err := h.service.GetByID(r.Context(), user.ID, generationID)
	if err != nil {
		if errors.Is(err, generation.ErrGenerationNotFound) {
			response.WriteError(w, r, http.StatusNotFound, "generation_not_found", "generation not found")
			return
		}

		logger := h.requestLogger(r)
		logger.Error().Err(err).Str("user_id", user.ID).Str("generation_id", generationID).Msg("не удалось получить статус генерации")
		response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to load generation status")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, toGenerationStatusResponse(result))
}

func validateCreateGenerationRequest(req openapi.CreateGenerationRequest) []openapi.ErrorDetail {
	var details []openapi.ErrorDetail

	if len(strings.TrimSpace(stringOrEmpty(req.ProjectName))) > 200 {
		details = append(details, openapi.ErrorDetail{Field: strPtr("project_name"), Message: "must be at most 200 characters"})
	}
	if strings.TrimSpace(req.MarketplaceId) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("marketplace_id"), Message: "field is required"})
	}
	if strings.TrimSpace(req.StyleId) == "" {
		details = append(details, openapi.ErrorDetail{Field: strPtr("style_id"), Message: "field is required"})
	}
	if len(req.CardTypeIds) == 0 {
		details = append(details, openapi.ErrorDetail{Field: strPtr("card_type_ids"), Message: "at least one card type is required"})
	}
	if req.CardCount <= 0 {
		details = append(details, openapi.ErrorDetail{Field: strPtr("card_count"), Message: "must be greater than zero"})
	}
	// SourceAssetId — типизированный UUID, нулевой UUID (00000000-...) означает что поле не передано.
	if req.SourceAssetId == [16]byte{} {
		details = append(details, openapi.ErrorDetail{Field: strPtr("source_asset_id"), Message: "field is required"})
	}

	return details
}

func toGenerateMarketplaces(items []generation.CatalogOption) []openapi.GenerateMarketplace {
	result := make([]openapi.GenerateMarketplace, len(items))
	for i, item := range items {
		result[i] = openapi.GenerateMarketplace{
			Id:    item.ID,
			Label: item.Label,
		}
	}

	return result
}

func toGenerateStyles(items []generation.CatalogOption) []openapi.GenerateStyle {
	result := make([]openapi.GenerateStyle, len(items))
	for i, item := range items {
		result[i] = openapi.GenerateStyle{
			Id:    item.ID,
			Label: item.Label,
		}
	}

	return result
}

func toGenerateCardTypes(items []generation.CardTypeOption) []openapi.GenerateCardType {
	result := make([]openapi.GenerateCardType, len(items))
	for i, item := range items {
		ct := openapi.GenerateCardType{
			Id:    item.ID,
			Label: item.Label,
		}
		if item.DefaultSelected {
			ct.DefaultSelected = &item.DefaultSelected
		}
		result[i] = ct
	}

	return result
}

func toGeneratedCards(items []generation.GeneratedCard) []openapi.GeneratedCard {
	result := make([]openapi.GeneratedCard, len(items))
	for i, item := range items {
		result[i] = openapi.GeneratedCard{
			Id:         mustParseUUID(item.ID),
			CardTypeId: item.CardTypeID,
			AssetId:    mustParseUUID(item.AssetID),
			PreviewUrl: item.PreviewURL,
		}
	}

	return result
}

// toGenerationStatusResponse конвертирует доменный Status в openapi.GenerationStatusResponse.
// Опциональные поля передаются как указатели — nil сериализуется с omitempty.
func toGenerationStatusResponse(result generation.Status) openapi.GenerationStatusResponse {
	resp := openapi.GenerationStatusResponse{
		GenerationId: mustParseUUID(result.GenerationID),
		Status:       openapi.GenerationStatusResponseStatus(result.Status),
	}
	if result.CurrentStep != "" {
		resp.CurrentStep = &result.CurrentStep
	}
	if result.ProgressPercent > 0 {
		resp.ProgressPercent = &result.ProgressPercent
	}
	if result.ErrorMessage != "" {
		resp.ErrorMessage = &result.ErrorMessage
	}
	if result.ArchiveDownloadURL != "" {
		resp.ArchiveDownloadUrl = &result.ArchiveDownloadURL
	}
	if len(result.ResultCards) > 0 {
		cards := toGeneratedCards(result.ResultCards)
		resp.ResultCards = &cards
	}
	return resp
}

func (h GenerationHandler) requestLogger(r *http.Request) zerolog.Logger {
	return requestctx.Logger(r.Context(), h.logger)
}
