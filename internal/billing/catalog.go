package billing

import (
	"strings"

	"kartochki-online-backend/internal/dbgen"
)

var planFeaturesByCode = map[string][]PlanFeature{
	"free": {
		{Label: "До 30 карточек в месяц", Enabled: true},
		{Label: "Базовые стили генерации", Enabled: true},
		{Label: "Экспорт архивом", Enabled: false},
	},
	"pro": {
		{Label: "До 500 карточек в месяц", Enabled: true},
		{Label: "Все стили генерации", Enabled: true},
		{Label: "Экспорт архивом", Enabled: true},
	},
	"business": {
		{Label: "До 2500 карточек в месяц", Enabled: true},
		{Label: "Командный запас по лимитам", Enabled: true},
		{Label: "Приоритет на будущие интеграции", Enabled: true},
	},
}

type planCatalogItem struct {
	Name               string
	MonthlyPrice       int
	YearlyMonthlyPrice int
	CardsPerMonth      int
	Popular            bool
	Features           []PlanFeature
}

var planCatalogByCode = map[string]planCatalogItem{
	"test": {
		Name:          "Триал",
		MonthlyPrice:  10,
		CardsPerMonth: 1,
		Features: []PlanFeature{
			{Label: "1 карточка в месяц", Enabled: true},
			{Label: "Тестовая оплата через ЮKassa", Enabled: true},
			{Label: "Без автопродления", Enabled: true},
		},
	},
	"free": {
		Name:          "Старт",
		MonthlyPrice:  0,
		CardsPerMonth: 5,
		Features: []PlanFeature{
			{Label: "Для теста сервиса и первых карточек товара", Enabled: true},
			{Label: "До 5 карточек в месяц", Enabled: true},
			{Label: "Генерация инфографики, фото и текстов", Enabled: true},
			{Label: "Подходит для знакомства, но не для постоянного потока", Enabled: false},
		},
	},
	"business": {
		Name:               "Бизнес",
		MonthlyPrice:       1490,
		YearlyMonthlyPrice: 1192,
		CardsPerMonth:      75,
		Popular:            true,
		Features: []PlanFeature{
			{Label: "Для регулярной работы селлера с каталогом", Enabled: true},
			{Label: "До 75 карточек в месяц", Enabled: true},
			{Label: "Полный доступ к генерации изображений и текстов", Enabled: true},
			{Label: "Без интеграции по API", Enabled: false},
		},
	},
	"agency": {
		Name:               "Агентство",
		MonthlyPrice:       4990,
		YearlyMonthlyPrice: 3992,
		CardsPerMonth:      250,
		Features: []PlanFeature{
			{Label: "Для агентств и команд с несколькими проектами", Enabled: true},
			{Label: "До 250 карточек в месяц", Enabled: true},
			{Label: "Полный доступ ко всем функциям сервиса", Enabled: true},
			{Label: "Приоритетная поддержка", Enabled: true},
			{Label: "Без интеграции по API", Enabled: false},
		},
	},
	"corporate": {
		Name:               "Корпоративный",
		MonthlyPrice:       14990,
		YearlyMonthlyPrice: 11992,
		CardsPerMonth:      750,
		Features: []PlanFeature{
			{Label: "Для крупных команд и потоковой генерации карточек", Enabled: true},
			{Label: "До 750 карточек в месяц", Enabled: true},
			{Label: "Полный доступ ко всем функциям сервиса", Enabled: true},
			{Label: "Приоритетная поддержка", Enabled: true},
			{Label: "Интеграция по API", Enabled: true},
		},
	},
}

func toPlans(rows []dbgen.Plan, currentPlanCode string) []Plan {
	result := make([]Plan, len(rows))
	for i, row := range rows {
		catalog, ok := planCatalogByCode[row.Code]
		name := strings.TrimSpace(row.Name)
		monthlyPrice := int(row.MonthlyPrice)
		yearlyMonthlyPrice := int32Value(row.YearlyMonthlyPrice)
		cardsPerMonth := int(row.CardsPerMonth)
		features := clonePlanFeatures(planFeaturesByCode[row.Code])
		popular := row.IsPopular

		if ok {
			name = catalog.Name
			monthlyPrice = catalog.MonthlyPrice
			yearlyMonthlyPrice = catalog.YearlyMonthlyPrice
			cardsPerMonth = catalog.CardsPerMonth
			features = clonePlanFeatures(catalog.Features)
			popular = catalog.Popular
		}

		result[i] = Plan{
			ID:                 row.Code,
			Name:               name,
			MonthlyPrice:       monthlyPrice,
			YearlyMonthlyPrice: yearlyMonthlyPrice,
			CardsPerMonth:      cardsPerMonth,
			Features:           features,
			Current:            row.Code == currentPlanCode,
			Popular:            popular,
		}
	}

	return result
}

func toAddons(rows []dbgen.AddonProduct) []Addon {
	result := make([]Addon, len(rows))
	for i, row := range rows {
		result[i] = Addon{
			ID:          row.Code,
			Title:       strings.TrimSpace(row.Title),
			Description: strings.TrimSpace(row.Description),
			Price:       int(row.Price),
		}
	}

	return result
}

func clonePlanFeatures(features []PlanFeature) []PlanFeature {
	if len(features) == 0 {
		return nil
	}

	result := make([]PlanFeature, len(features))
	copy(result, features)
	return result
}
