package projects

import "time"

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
	Cards              []ProjectCard
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
