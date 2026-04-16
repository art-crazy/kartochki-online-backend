package projects

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"kartochki-online-backend/internal/dbgen"
)

const dashboardPreviewLimit = 4

// ProjectCard описывает одну готовую карточку проекта без HTTP-деталей.
type ProjectCard struct {
	ID         string
	CardTypeID string
	AssetID    string
	PreviewURL string
}

// projectStorage даёт projects-сервису только то, что нужно для сборки публичных URL карточек.
type projectStorage interface {
	PublicURL(storageKey string) string
}

func (s *Service) listCompletedCardsByProject(ctx context.Context, projectID string, userID string) ([]ProjectCard, error) {
	parsedProjectID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, ErrNotFound
	}

	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return nil, ErrNotFound
	}

	rows, err := s.queries.ListCompletedCardsByProjectID(ctx, dbgen.ListCompletedCardsByProjectIDParams{
		ProjectID: parsedProjectID,
		UserID:    parsedUserID,
	})
	if err != nil {
		return nil, fmt.Errorf("list completed project cards: %w", err)
	}

	return s.toProjectCards(rows), nil
}

func (s *Service) listCompletedCardsByUser(ctx context.Context, userID string) (map[string][]ProjectCard, error) {
	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("parse user id: %w", err)
	}

	rows, err := s.queries.ListCompletedProjectCards(ctx, parsedUserID)
	if err != nil {
		return nil, fmt.Errorf("list completed cards for dashboard: %w", err)
	}

	grouped := make(map[string][]ProjectCard, len(rows))
	for _, row := range rows {
		projectID := row.ProjectID.String()
		grouped[projectID] = append(grouped[projectID], ProjectCard{
			ID:         row.ID.String(),
			CardTypeID: row.CardTypeID,
			AssetID:    row.AssetID.String(),
			PreviewURL: s.storage.PublicURL(row.StorageKey),
		})
	}

	return grouped, nil
}

func (s *Service) toProjectCards(rows []dbgen.ListCompletedCardsByProjectIDRow) []ProjectCard {
	result := make([]ProjectCard, len(rows))
	for i, row := range rows {
		result[i] = ProjectCard{
			ID:         row.ID.String(),
			CardTypeID: row.CardTypeID,
			AssetID:    row.AssetID.String(),
			PreviewURL: s.storage.PublicURL(row.StorageKey),
		}
	}

	return result
}

func buildDashboardProjects(list []Project) []DashboardProject {
	result := make([]DashboardProject, len(list))
	for i, p := range list {
		previewCap := len(p.Cards)
		if previewCap > dashboardPreviewLimit {
			previewCap = dashboardPreviewLimit
		}
		previewURLs := make([]string, 0, previewCap)
		for j, card := range p.Cards {
			if j >= dashboardPreviewLimit {
				break
			}
			previewURLs = append(previewURLs, card.PreviewURL)
		}

		result[i] = DashboardProject{
			ID:            p.ID,
			Title:         p.Title,
			CardCount:     len(p.Cards),
			MarketplaceID: p.Marketplace,
			UpdatedAt:     p.UpdatedAt,
			PreviewURLs:   previewURLs,
		}
	}

	return result
}
