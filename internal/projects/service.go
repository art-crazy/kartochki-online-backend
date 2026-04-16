package projects

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kartochki-online-backend/internal/dbgen"
)

// Service управляет сценариями работы с проектами и не знает о деталях HTTP.
type Service struct {
	queries *dbgen.Queries
	storage projectStorage
}

// NewService создаёт сервис проектов.
func NewService(queries *dbgen.Queries, storage projectStorage) *Service {
	return &Service{queries: queries, storage: storage}
}

// GetDashboard возвращает данные для `/app` без HTTP-деталей.
func (s *Service) GetDashboard(ctx context.Context, userID string) (Dashboard, error) {
	allProjects, err := s.ListByUser(ctx, userID)
	if err != nil {
		return Dashboard{}, fmt.Errorf("list projects for dashboard: %w", err)
	}

	projectCards, err := s.listCompletedCardsByUser(ctx, userID)
	if err != nil {
		return Dashboard{}, err
	}
	allProjects = attachCardsToProjects(allProjects, projectCards)

	recentProjects := allProjects
	if len(recentProjects) > recentProjectsLimit {
		recentProjects = recentProjects[:recentProjectsLimit]
	}

	totalProjects := len(allProjects)
	return Dashboard{
		Stats:          buildDashboardStats(totalProjects),
		RecentProjects: buildDashboardProjects(recentProjects),
		AllProjects:    buildDashboardProjects(allProjects),
		QuickStart:     buildQuickStart(totalProjects),
	}, nil
}

// Create создаёт проект и возвращает его.
func (s *Service) Create(ctx context.Context, input CreateInput) (Project, error) {
	input = normalizeCreateInput(input)
	if err := validateCreateOrUpdateInput(input.Title, input.Marketplace, input.ProductName, input.ProductDescription); err != nil {
		return Project{}, err
	}

	userID, err := uuid.Parse(input.UserID)
	if err != nil {
		return Project{}, fmt.Errorf("parse user id: %w", err)
	}

	row, err := s.queries.CreateProject(ctx, dbgen.CreateProjectParams{
		UserID:             userID,
		Title:              input.Title,
		Marketplace:        input.Marketplace,
		ProductName:        input.ProductName,
		ProductDescription: input.ProductDescription,
	})
	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}

	return toProject(row), nil
}

// GetByID возвращает проект по id. Если проект не найден или не принадлежит ownerUserID — ErrNotFound.
func (s *Service) GetByID(ctx context.Context, id string, ownerUserID string) (Project, error) {
	projectID, err := uuid.Parse(id)
	if err != nil {
		return Project{}, ErrNotFound
	}

	userID, err := uuid.Parse(ownerUserID)
	if err != nil {
		return Project{}, ErrNotFound
	}

	row, err := s.queries.GetProjectByID(ctx, dbgen.GetProjectByIDParams{
		ID:     projectID,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Project{}, ErrNotFound
		}

		return Project{}, fmt.Errorf("get project by id: %w", err)
	}

	project := toProject(row)
	project.Cards, err = s.listCompletedCardsByProject(ctx, project.ID, ownerUserID)
	if err != nil {
		return Project{}, err
	}

	return project, nil
}

// ListByUser возвращает все активные проекты пользователя, отсортированные по дате обновления.
func (s *Service) ListByUser(ctx context.Context, userID string) ([]Project, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("parse user id: %w", err)
	}

	rows, err := s.queries.ListUserProjects(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("list user projects: %w", err)
	}

	return toProjects(rows), nil
}

// Update обновляет поля проекта. user_id проверяется на уровне SQL одним запросом к БД.
func (s *Service) Update(ctx context.Context, id string, ownerUserID string, input UpdateInput) (Project, error) {
	input = normalizeUpdateInput(input)
	if err := validateCreateOrUpdateInput(input.Title, input.Marketplace, input.ProductName, input.ProductDescription); err != nil {
		return Project{}, err
	}

	projectID, err := uuid.Parse(id)
	if err != nil {
		return Project{}, ErrNotFound
	}

	userID, err := uuid.Parse(ownerUserID)
	if err != nil {
		return Project{}, ErrNotFound
	}

	row, err := s.queries.UpdateProject(ctx, dbgen.UpdateProjectParams{
		ID:                 projectID,
		UserID:             userID,
		Title:              input.Title,
		Marketplace:        input.Marketplace,
		ProductName:        input.ProductName,
		ProductDescription: input.ProductDescription,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Project{}, ErrNotFound
		}

		return Project{}, fmt.Errorf("update project: %w", err)
	}

	return toProject(row), nil
}

// Patch частично обновляет проект.
func (s *Service) Patch(ctx context.Context, id string, ownerUserID string, input PatchInput) (Project, error) {
	current, err := s.GetByID(ctx, id, ownerUserID)
	if err != nil {
		return Project{}, err
	}

	update := UpdateInput{
		Title:              current.Title,
		Marketplace:        current.Marketplace,
		ProductName:        current.ProductName,
		ProductDescription: current.ProductDescription,
	}

	if input.Title != nil {
		update.Title = *input.Title
	}
	if input.Marketplace != nil {
		update.Marketplace = *input.Marketplace
	}
	if input.ProductName != nil {
		update.ProductName = *input.ProductName
	}
	if input.ProductDescription != nil {
		update.ProductDescription = *input.ProductDescription
	}

	return s.Update(ctx, id, ownerUserID, update)
}

// Delete мягко удаляет проект пользователя.
func (s *Service) Delete(ctx context.Context, id string, ownerUserID string) error {
	projectID, err := uuid.Parse(id)
	if err != nil {
		return ErrNotFound
	}

	userID, err := uuid.Parse(ownerUserID)
	if err != nil {
		return ErrNotFound
	}

	rows, err := s.queries.SoftDeleteProject(ctx, dbgen.SoftDeleteProjectParams{
		ID:     projectID,
		UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("soft delete project: %w", err)
	}

	if rows == 0 {
		return ErrNotFound
	}

	return nil
}
