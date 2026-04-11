package contracts

import "time"

// DashboardResponse описывает данные для страницы `/app`.
type DashboardResponse struct {
	Stats          []DashboardStat     `json:"stats"`
	RecentProjects []DashboardProject  `json:"recent_projects"`
	AllProjects    []DashboardProject  `json:"all_projects"`
	QuickStart     DashboardQuickStart `json:"quick_start"`
}

// DashboardStat описывает один показатель в верхнем блоке дашборда.
type DashboardStat struct {
	Key         string             `json:"key"`
	Label       string             `json:"label"`
	Value       string             `json:"value"`
	Description string             `json:"description"`
	AccentText  string             `json:"accent_text,omitempty"`
	Progress    *DashboardProgress `json:"progress,omitempty"`
}

// DashboardProgress описывает прогресс по лимиту или квоте.
type DashboardProgress struct {
	Value int `json:"value"`
	Max   int `json:"max"`
}

// DashboardProject описывает проект в списке или в блоке последних проектов.
type DashboardProject struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	CardCount     int       `json:"card_count"`
	MarketplaceID string    `json:"marketplace_id"`
	UpdatedAt     time.Time `json:"updated_at"`
	PreviewURLs   []string  `json:"preview_urls,omitempty"`
	CanonicalPath string    `json:"canonical_path"`
}

// DashboardQuickStart описывает CTA-блок быстрого старта.
type DashboardQuickStart struct {
	Title         string `json:"title"`
	Description   string `json:"description"`
	CanonicalPath string `json:"canonical_path"`
}
