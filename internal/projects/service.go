package projects

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kartochki-online-backend/internal/dbgen"
)

const (
	recentProjectsLimit = 5
)

// MaxProjectTitleLength ограничивает длину названия проекта.
const MaxProjectTitleLength = 200

// MaxMarketplaceLength ограничивает длину идентификатора маркетплейса.
const MaxMarketplaceLength = 100

// MaxProjectProductNameLength ограничивает длину названия товара внутри проекта.
const MaxProjectProductNameLength = 255

// MaxProjectDescriptionLength ограничивает длину описания товара внутри проекта.
const MaxProjectDescriptionLength = 5000

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

// UpdateInput содержит поля для полного обновления проекта.
type UpdateInput struct {
	Title              string
	Marketplace        string
	ProductName        string
	ProductDescription string
}

// PatchInput содержит только те поля проекта, которые клиент действительно хочет изменить.
// nil означает, что поле нужно оставить без изменений.
type PatchInput struct {
	Title              *string
	Marketplace        *string
	ProductName        *string
	ProductDescription *string
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
// Здесь собирается бизнес-ответ для дашборда, чтобы handler только проверял доступ
// и преобразовывал результат в transport-контракт.
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

	return toProject(row), nil
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
// Сначала сервис читает текущую версию проекта владельца, чтобы не затирать поля,
// которые не пришли в PATCH-запросе.
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
// Мы не стираем строку физически, чтобы позже не потерять историю генераций,
// файлов и других связанных сущностей, которые будут ссылаться на проект.
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
			Description: "Загрузите фото товара - мы сгенерируем карточки для маркетплейса",
		}
	}

	return DashboardQuickStart{
		Title:       "Сгенерировать новые карточки",
		Description: "Загрузите новое фото и получите готовые карточки",
	}
}

func normalizeCreateInput(input CreateInput) CreateInput {
	input.UserID = strings.TrimSpace(input.UserID)
	input.Title = strings.TrimSpace(input.Title)
	input.Marketplace = strings.TrimSpace(input.Marketplace)
	input.ProductName = strings.TrimSpace(input.ProductName)
	input.ProductDescription = strings.TrimSpace(input.ProductDescription)
	return input
}

func normalizeUpdateInput(input UpdateInput) UpdateInput {
	input.Title = strings.TrimSpace(input.Title)
	input.Marketplace = strings.TrimSpace(input.Marketplace)
	input.ProductName = strings.TrimSpace(input.ProductName)
	input.ProductDescription = strings.TrimSpace(input.ProductDescription)
	return input
}

// validateCreateOrUpdateInput держит доменные ограничения в одном месте,
// чтобы Create и Update не разъезжались по правилам.
func validateCreateOrUpdateInput(title string, marketplace string, productName string, productDescription string) error {
	if title == "" {
		return ErrTitleRequired
	}
	if len(title) > MaxProjectTitleLength {
		return ErrTitleTooLong
	}
	if len(marketplace) > MaxMarketplaceLength {
		return ErrMarketplaceTooLong
	}
	if len(productName) > MaxProjectProductNameLength {
		return ErrProductNameTooLong
	}
	if len(productDescription) > MaxProjectDescriptionLength {
		return ErrProductDescriptionTooLong
	}

	return nil
}
