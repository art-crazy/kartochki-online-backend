package handlers

import (
	"context"
	"net/http"

	"github.com/rs/zerolog"

	openapi "kartochki-online-backend/api/gen"
	"kartochki-online-backend/internal/http/requestctx"
	"kartochki-online-backend/internal/http/response"
	"kartochki-online-backend/internal/projects"
)

const dashboardQuickStartPath = "/app/generate"

// dashboardProjectService описывает минимальный контракт получения данных дашборда.
type dashboardProjectService interface {
	GetDashboard(ctx context.Context, userID string) (projects.Dashboard, error)
}

// DashboardHandler обслуживает GET /api/v1/dashboard.
type DashboardHandler struct {
	projectService dashboardProjectService
	logger         zerolog.Logger
}

// NewDashboardHandler создаёт обработчик дашборда.
func NewDashboardHandler(projectService dashboardProjectService, logger zerolog.Logger) DashboardHandler {
	return DashboardHandler{projectService: projectService, logger: logger}
}

// Get возвращает данные для главной страницы приложения /app.
func (h DashboardHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUserFromCtx(w, r)
	if !ok {
		return
	}

	dashboard, err := h.projectService.GetDashboard(r.Context(), user.ID)
	if err != nil {
		logger := requestctx.Logger(r.Context(), h.logger)
		logger.Error().Err(err).Str("user_id", user.ID).Msg("не удалось загрузить проекты для дашборда")
		response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to load dashboard")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, toDashboardResponse(dashboard))
}

func toDashboardResponse(dashboard projects.Dashboard) openapi.DashboardResponse {
	return openapi.DashboardResponse{
		Stats:          toDashboardStats(dashboard.Stats),
		RecentProjects: toDashboardProjects(dashboard.RecentProjects),
		AllProjects:    toDashboardProjects(dashboard.AllProjects),
		QuickStart: openapi.DashboardQuickStart{
			Title:         dashboard.QuickStart.Title,
			Description:   dashboard.QuickStart.Description,
			CanonicalPath: dashboardQuickStartPath,
		},
	}
}

func toDashboardStats(stats []projects.DashboardStat) []openapi.DashboardStat {
	result := make([]openapi.DashboardStat, len(stats))
	for i, stat := range stats {
		s := openapi.DashboardStat{
			Key:         stat.Key,
			Label:       stat.Label,
			Value:       stat.Value,
			Description: stat.Description,
		}
		if stat.AccentText != "" {
			s.AccentText = &stat.AccentText
		}
		if stat.Progress != nil {
			s.Progress = &struct {
				Max   *int `json:"max,omitempty"`
				Value *int `json:"value,omitempty"`
			}{
				Value: &stat.Progress.Value,
				Max:   &stat.Progress.Max,
			}
		}
		result[i] = s
	}

	return result
}

func toDashboardProjects(list []projects.DashboardProject) []openapi.DashboardProject {
	result := make([]openapi.DashboardProject, len(list))
	for i, p := range list {
		id := mustParseUUID(p.ID)
		proj := openapi.DashboardProject{
			Id:            id,
			Title:         p.Title,
			UpdatedAt:     p.UpdatedAt,
			CanonicalPath: "/app/projects/" + p.ID,
		}
		if p.CardCount > 0 {
			proj.CardCount = &p.CardCount
		}
		if p.MarketplaceID != "" {
			proj.MarketplaceId = &p.MarketplaceID
		}
		if len(p.PreviewURLs) > 0 {
			urls := p.PreviewURLs
			proj.PreviewUrls = &urls
		}
		result[i] = proj
	}

	return result
}
