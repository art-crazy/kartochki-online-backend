package projects

import "strconv"

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

func attachCardsToProjects(projects []Project, cardsByProjectID map[string][]ProjectCard) []Project {
	result := make([]Project, len(projects))
	for i, project := range projects {
		project.Cards = cardsByProjectID[project.ID]
		result[i] = project
	}

	return result
}
