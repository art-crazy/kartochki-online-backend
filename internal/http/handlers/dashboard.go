package handlers

import (
	"net/http"

	"github.com/rs/zerolog"

	"kartochki-online-backend/internal/http/authctx"
	"kartochki-online-backend/internal/http/contracts"
	"kartochki-online-backend/internal/http/response"
	"kartochki-online-backend/internal/projects"
)

const dashboardQuickStartPath = "/app/generate"

// DashboardHandler обслуживает GET /api/v1/dashboard.
type DashboardHandler struct {
	projectService *projects.Service
	logger         zerolog.Logger
}

// NewDashboardHandler создаёт обработчик дашборда.
func NewDashboardHandler(projectService *projects.Service, logger zerolog.Logger) DashboardHandler {
	return DashboardHandler{projectService: projectService, logger: logger}
}

// Get возвращает данные для главной страницы приложения /app.
func (h DashboardHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := authctx.User(r.Context())
	if !ok {
		response.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "authorization token is invalid")
		return
	}

	dashboard, err := h.projectService.GetDashboard(r.Context(), user.ID)
	if err != nil {
		h.logger.Error().Err(err).Str("user_id", user.ID).Msg("не удалось загрузить проекты для дашборда")
		response.WriteError(w, r, http.StatusInternalServerError, "internal_error", "failed to load dashboard")
		return
	}

	response.WriteJSON(w, r, http.StatusOK, toDashboardResponse(dashboard))
}

func toDashboardResponse(dashboard projects.Dashboard) contracts.DashboardResponse {
	return contracts.DashboardResponse{
		Stats:          toDashboardStats(dashboard.Stats),
		RecentProjects: toDashboardProjects(dashboard.RecentProjects),
		AllProjects:    toDashboardProjects(dashboard.AllProjects),
		QuickStart: contracts.DashboardQuickStart{
			Title:         dashboard.QuickStart.Title,
			Description:   dashboard.QuickStart.Description,
			CanonicalPath: dashboardQuickStartPath,
		},
	}
}

func toDashboardStats(stats []projects.DashboardStat) []contracts.DashboardStat {
	result := make([]contracts.DashboardStat, len(stats))
	for i, stat := range stats {
		var progress *contracts.DashboardProgress
		if stat.Progress != nil {
			progress = &contracts.DashboardProgress{
				Value: stat.Progress.Value,
				Max:   stat.Progress.Max,
			}
		}

		result[i] = contracts.DashboardStat{
			Key:         stat.Key,
			Label:       stat.Label,
			Value:       stat.Value,
			Description: stat.Description,
			AccentText:  stat.AccentText,
			Progress:    progress,
		}
	}

	return result
}

func toDashboardProjects(list []projects.DashboardProject) []contracts.DashboardProject {
	result := make([]contracts.DashboardProject, len(list))
	for i, p := range list {
		result[i] = contracts.DashboardProject{
			ID:            p.ID,
			Title:         p.Title,
			CardCount:     p.CardCount,
			MarketplaceID: p.MarketplaceID,
			UpdatedAt:     p.UpdatedAt,
			PreviewURLs:   p.PreviewURLs,
			CanonicalPath: "/app/projects/" + p.ID,
		}
	}

	return result
}
