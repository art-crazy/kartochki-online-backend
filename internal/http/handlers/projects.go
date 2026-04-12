package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"kartochki-online-backend/internal/auth"
	"kartochki-online-backend/internal/http/authctx"
	"kartochki-online-backend/internal/http/contracts"
	"kartochki-online-backend/internal/http/requestctx"
	"kartochki-online-backend/internal/http/response"
	"kartochki-online-backend/internal/projects"
)

// projectService описывает бизнес-операции с проектами, которые нужны HTTP-слою.
type projectService interface {
	Create(ctx context.Context, input projects.CreateInput) (projects.Project, error)
	ListByUser(ctx context.Context, userID string) ([]projects.Project, error)
	GetByID(ctx context.Context, id string, ownerUserID string) (projects.Project, error)
	Patch(ctx context.Context, id string, ownerUserID string, input projects.PatchInput) (projects.Project, error)
	Delete(ctx context.Context, id string, ownerUserID string) error
}

// ProjectsHandler обслуживает CRUD-маршруты проектов пользователя.
type ProjectsHandler struct {
	projectService projectService
	logger         zerolog.Logger
}

// NewProjectsHandler создаёт обработчик endpoint проектов.
func NewProjectsHandler(projectService projectService, logger zerolog.Logger) ProjectsHandler {
	return ProjectsHandler{
		projectService: projectService,
		logger:         logger,
	}
}

// Create создаёт новый проект для текущего пользователя.
func (h ProjectsHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentUser(w, r)
	if !ok {
		return
	}

	var req contracts.CreateProjectRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if details := validateCreateProjectRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", details...)
		return
	}

	project, err := h.projectService.Create(r.Context(), projects.CreateInput{
		UserID:             user.ID,
		Title:              req.Title,
		Marketplace:        req.Marketplace,
		ProductName:        req.ProductName,
		ProductDescription: req.ProductDescription,
	})
	if err != nil {
		if writeProjectDomainError(w, r, err) {
			return
		}

		logger := h.requestLogger(r)
		logger.Error().Err(err).Str("user_id", user.ID).Msg("не удалось создать проект")
		response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to create project")
		return
	}

	response.WriteJSON(w, r, http.StatusCreated, contracts.ProjectResponse{Project: toProjectContract(project)})
}

// List возвращает все активные проекты текущего пользователя.
func (h ProjectsHandler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentUser(w, r)
	if !ok {
		return
	}

	list, err := h.projectService.ListByUser(r.Context(), user.ID)
	if err != nil {
		logger := h.requestLogger(r)
		logger.Error().Err(err).Str("user_id", user.ID).Msg("не удалось загрузить список проектов")
		response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to load projects")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, contracts.ProjectListResponse{Projects: toProjectContracts(list)})
}

// Get возвращает один активный проект текущего пользователя.
func (h ProjectsHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentUser(w, r)
	if !ok {
		return
	}

	projectID := chi.URLParam(r, "id")
	project, err := h.projectService.GetByID(r.Context(), projectID, user.ID)
	if err != nil {
		if errors.Is(err, projects.ErrNotFound) {
			response.WriteError(w, r, http.StatusNotFound, "project_not_found", "project not found")
			return
		}

		logger := h.requestLogger(r)
		logger.Error().Err(err).Str("user_id", user.ID).Str("project_id", projectID).Msg("не удалось загрузить проект")
		response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to load project")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, contracts.ProjectResponse{Project: toProjectContract(project)})
}

// Patch частично обновляет проект текущего пользователя.
func (h ProjectsHandler) Patch(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentUser(w, r)
	if !ok {
		return
	}

	var req contracts.PatchProjectRequest
	if err := decodeJSON(r, &req); err != nil {
		response.WriteError(w, r, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	if details := validatePatchProjectRequest(req); len(details) > 0 {
		response.WriteError(w, r, http.StatusBadRequest, "validation_error", "request validation failed", details...)
		return
	}

	projectID := chi.URLParam(r, "id")
	project, err := h.projectService.Patch(r.Context(), projectID, user.ID, projects.PatchInput{
		Title:              req.Title,
		Marketplace:        req.Marketplace,
		ProductName:        req.ProductName,
		ProductDescription: req.ProductDescription,
	})
	if err != nil {
		if errors.Is(err, projects.ErrNotFound) {
			response.WriteError(w, r, http.StatusNotFound, "project_not_found", "project not found")
			return
		}
		if writeProjectDomainError(w, r, err) {
			return
		}

		logger := h.requestLogger(r)
		logger.Error().Err(err).Str("user_id", user.ID).Str("project_id", projectID).Msg("не удалось обновить проект")
		response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to update project")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, contracts.ProjectResponse{Project: toProjectContract(project)})
}

// Delete мягко удаляет проект текущего пользователя.
func (h ProjectsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, ok := h.currentUser(w, r)
	if !ok {
		return
	}

	projectID := chi.URLParam(r, "id")
	if err := h.projectService.Delete(r.Context(), projectID, user.ID); err != nil {
		if errors.Is(err, projects.ErrNotFound) {
			response.WriteError(w, r, http.StatusNotFound, "project_not_found", "project not found")
			return
		}

		logger := h.requestLogger(r)
		logger.Error().Err(err).Str("user_id", user.ID).Str("project_id", projectID).Msg("не удалось удалить проект")
		response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to delete project")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, contracts.StatusResponse{Status: "deleted"})
}

func validateCreateProjectRequest(req contracts.CreateProjectRequest) []contracts.ErrorDetail {
	return validateProjectFields(req.Title, req.Marketplace, req.ProductName, req.ProductDescription, true)
}

func validatePatchProjectRequest(req contracts.PatchProjectRequest) []contracts.ErrorDetail {
	var details []contracts.ErrorDetail

	if req.Title == nil && req.Marketplace == nil && req.ProductName == nil && req.ProductDescription == nil {
		details = append(details, contracts.ErrorDetail{Message: "at least one field must be provided"})
		return details
	}

	title := ""
	marketplace := ""
	productName := ""
	productDescription := ""

	if req.Title != nil {
		title = *req.Title
	}
	if req.Marketplace != nil {
		marketplace = *req.Marketplace
	}
	if req.ProductName != nil {
		productName = *req.ProductName
	}
	if req.ProductDescription != nil {
		productDescription = *req.ProductDescription
	}

	return validateProjectFields(title, marketplace, productName, productDescription, false)
}

// validateProjectFields повторяет transport-валидацию до вызова сервиса,
// чтобы клиент сразу получил понятную ошибку по полю, а не общий отказ домена.
func validateProjectFields(title string, marketplace string, productName string, productDescription string, titleRequired bool) []contracts.ErrorDetail {
	var details []contracts.ErrorDetail

	trimmedTitle := strings.TrimSpace(title)
	if titleRequired && trimmedTitle == "" {
		details = append(details, contracts.ErrorDetail{Field: "title", Message: "field is required"})
	}
	if title != "" && trimmedTitle == "" {
		details = append(details, contracts.ErrorDetail{Field: "title", Message: "must not be empty"})
	}
	if len(trimmedTitle) > projects.MaxProjectTitleLength {
		details = append(details, contracts.ErrorDetail{Field: "title", Message: "must be at most 200 characters"})
	}

	if len(strings.TrimSpace(marketplace)) > projects.MaxMarketplaceLength {
		details = append(details, contracts.ErrorDetail{Field: "marketplace", Message: "must be at most 100 characters"})
	}
	if len(strings.TrimSpace(productName)) > projects.MaxProjectProductNameLength {
		details = append(details, contracts.ErrorDetail{Field: "product_name", Message: "must be at most 255 characters"})
	}
	if len(strings.TrimSpace(productDescription)) > projects.MaxProjectDescriptionLength {
		details = append(details, contracts.ErrorDetail{Field: "product_description", Message: "must be at most 5000 characters"})
	}

	return details
}

func writeProjectDomainError(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case errors.Is(err, projects.ErrTitleRequired):
		response.WriteError(
			w,
			r,
			http.StatusBadRequest,
			"validation_error",
			"request validation failed",
			contracts.ErrorDetail{Field: "title", Message: "field is required"},
		)
		return true
	case errors.Is(err, projects.ErrTitleTooLong):
		response.WriteError(
			w,
			r,
			http.StatusBadRequest,
			"validation_error",
			"request validation failed",
			contracts.ErrorDetail{Field: "title", Message: "must be at most 200 characters"},
		)
		return true
	case errors.Is(err, projects.ErrMarketplaceTooLong):
		response.WriteError(
			w,
			r,
			http.StatusBadRequest,
			"validation_error",
			"request validation failed",
			contracts.ErrorDetail{Field: "marketplace", Message: "must be at most 100 characters"},
		)
		return true
	case errors.Is(err, projects.ErrProductNameTooLong):
		response.WriteError(
			w,
			r,
			http.StatusBadRequest,
			"validation_error",
			"request validation failed",
			contracts.ErrorDetail{Field: "product_name", Message: "must be at most 255 characters"},
		)
		return true
	case errors.Is(err, projects.ErrProductDescriptionTooLong):
		response.WriteError(
			w,
			r,
			http.StatusBadRequest,
			"validation_error",
			"request validation failed",
			contracts.ErrorDetail{Field: "product_description", Message: "must be at most 5000 characters"},
		)
		return true
	default:
		return false
	}
}

func toProjectContract(project projects.Project) contracts.Project {
	return contracts.Project{
		ID:                 project.ID,
		Title:              project.Title,
		Marketplace:        project.Marketplace,
		ProductName:        project.ProductName,
		ProductDescription: project.ProductDescription,
		Status:             project.Status,
		CreatedAt:          project.CreatedAt,
		UpdatedAt:          project.UpdatedAt,
	}
}

func toProjectContracts(list []projects.Project) []contracts.Project {
	result := make([]contracts.Project, len(list))
	for i, project := range list {
		result[i] = toProjectContract(project)
	}

	return result
}

// currentUser возвращает пользователя из auth middleware.
// Если middleware по какой-то причине не положил пользователя в context, endpoint
// отвечает так же, как и остальные защищённые маршруты проекта.
func (h ProjectsHandler) currentUser(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	user, ok := authctx.User(r.Context())
	if !ok {
		response.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "authorization token is invalid")
		return auth.User{}, false
	}

	return user, true
}

// requestLogger возвращает request-scoped logger, чтобы ошибки handler уже содержали request_id и путь.
func (h ProjectsHandler) requestLogger(r *http.Request) zerolog.Logger {
	return requestctx.Logger(r.Context(), h.logger)
}
