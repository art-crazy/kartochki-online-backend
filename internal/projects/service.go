package projects

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kartochki-online-backend/internal/dbgen"
)

const recentProjectsLimit = 5

// Project описывает проект пользователя без HTTP-деталей.
type Project struct {
	ID                 string
	UserID             string
	Title              string
	Marketplace        string
	ProductName        string
	ProductDescription string
	Status             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Dashboard описывает данные для главной страницы приложения.
type Dashboard struct {
	Stats          []DashboardStat
	RecentProjects []DashboardProject
	AllProjects    []DashboardProject
	QuickStart     DashboardQuickStart
}

// DashboardStat описывает один показатель в верхнем блоке дашборда.
type DashboardStat struct {
	Key         string
	Label       string
	Value       string
	Description string
	AccentText  string
	Progress    *DashboardProgress
}

// DashboardProgress описывает прогресс по лимиту или квоте.
type DashboardProgress struct {
	Value int
	Max   int
}

// DashboardProject описывает проект для списков дашборда.
type DashboardProject struct {
	ID            string
	Title         string
	CardCount     int
	MarketplaceID string
	UpdatedAt     time.Time
	PreviewURLs   []string
}

// DashboardQuickStart описывает CTA-блок быстрого старта.
type DashboardQuickStart struct {
	Title       string
	Description string
}

// CreateInput содержит данные для создания проекта.
type CreateInput struct {
	UserID             string
	Title              string
	Marketplace        string
	ProductName        string
	ProductDescription string
}

// UpdateInput содержит поля для обновления проекта.
type UpdateInput struct {
	Title              string
	Marketplace        string
	ProductName        string
	ProductDescription string
}

// Service управляет сценариями работы с проектами и не знает о деталях HTTP.
type Service struct {
	queries *dbgen.Queries
}

// NewService создаёт сервис проектов.
func NewService(queries *dbgen.Queries) *Service {
	return &Service{queries: queries}
}

// GetDashboard возвращает данные для `/app` без HTTP-деталей.
// Здесь собирается бизнес-ответ для дашборда, чтобы handler только
// проверял доступ и преобразовывал результат в transport-контракт.
func (s *Service) GetDashboard(ctx context.Context, userID string) (Dashboard, error) {
	allProjects, err := s.ListByUser(ctx, userID)
	if err != nil {
		return Dashboard{}, fmt.Errorf("list projects for dashboard: %w", err)
	}

	recentProjects := allProjects
	if len(recentProjects) > recentProjectsLimit {
		recentProjects = recentProjects[:recentProjectsLimit]
	}

	totalProjects := len(allProjects)
	return Dashboard{
		Stats:          buildDashboardStats(totalProjects),
		RecentProjects: toDashboardProjects(recentProjects),
		AllProjects:    toDashboardProjects(allProjects),
		QuickStart:     buildQuickStart(totalProjects),
	}, nil
}

// Create создаёт проект и возвращает его.
func (s *Service) Create(ctx context.Context, input CreateInput) (Project, error) {
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

	return toProject(row), nil
}

// ListByUser возвращает все проекты пользователя, отсортированные по дате обновления.
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

// Update обновляет поля проекта. user_id проверяется на уровне SQL — один запрос к БД.
func (s *Service) Update(ctx context.Context, id string, ownerUserID string, input UpdateInput) (Project, error) {
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

// Delete удаляет проект пользователя. Возвращает ErrNotFound, если проект не найден или чужой.
func (s *Service) Delete(ctx context.Context, id string, ownerUserID string) error {
	projectID, err := uuid.Parse(id)
	if err != nil {
		return ErrNotFound
	}

	userID, err := uuid.Parse(ownerUserID)
	if err != nil {
		return ErrNotFound
	}

	rows, err := s.queries.DeleteProject(ctx, dbgen.DeleteProjectParams{
		ID:     projectID,
		UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}

	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

func toProject(r dbgen.Project) Project {
	return Project{
		ID:                 r.ID.String(),
		UserID:             r.UserID.String(),
		Title:              r.Title,
		Marketplace:        r.Marketplace,
		ProductName:        r.ProductName,
		ProductDescription: r.ProductDescription,
		Status:             r.Status,
		CreatedAt:          r.CreatedAt.Time,
		UpdatedAt:          r.UpdatedAt.Time,
	}
}

func toProjects(rows []dbgen.Project) []Project {
	result := make([]Project, len(rows))
	for i, r := range rows {
		result[i] = toProject(r)
	}

	return result
}

func buildDashboardStats(totalProjects int) []DashboardStat {
	return []DashboardStat{
		{
			Key:         "total_projects",
			Label:       "Всего проектов",
			Value:       strconv.Itoa(totalProjects),
			Description: "проектов создано",
		},
	}
}

func toDashboardProjects(list []Project) []DashboardProject {
	result := make([]DashboardProject, len(list))
	for i, p := range list {
		result[i] = DashboardProject{
			ID:            p.ID,
			Title:         p.Title,
			MarketplaceID: p.Marketplace,
			UpdatedAt:     p.UpdatedAt,
		}
	}

	return result
}

// buildQuickStart формирует CTA-блок на главной.
// Когда проектов ещё нет, важно подсказать первый целевой сценарий.
func buildQuickStart(totalProjects int) DashboardQuickStart {
	if totalProjects == 0 {
		return DashboardQuickStart{
			Title:       "Создайте первый проект",
			Description: "Загрузите фото товара — мы сгенерируем карточки для маркетплейса",
		}
	}

	return DashboardQuickStart{
		Title:       "Сгенерировать новые карточки",
		Description: "Загрузите новое фото и получите готовые карточки",
	}
}
