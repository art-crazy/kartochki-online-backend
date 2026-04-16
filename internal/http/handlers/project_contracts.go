package handlers

import (
	openapi "kartochki-online-backend/api/gen"
	"kartochki-online-backend/internal/projects"
)

// toProjectContract конвертирует доменный projects.Project в openapi.Project для HTTP-ответа.
func toProjectContract(project projects.Project) openapi.Project {
	p := openapi.Project{
		Id:        mustParseUUID(project.ID),
		Title:     project.Title,
		Status:    openapi.ProjectStatus(project.Status),
		CreatedAt: project.CreatedAt,
		UpdatedAt: project.UpdatedAt,
	}
	if project.Marketplace != "" {
		p.Marketplace = &project.Marketplace
	}
	if project.ProductName != "" {
		p.ProductName = &project.ProductName
	}
	if project.ProductDescription != "" {
		p.ProductDescription = &project.ProductDescription
	}

	cards := make([]openapi.GeneratedCard, len(project.Cards))
	for i, card := range project.Cards {
		cards[i] = openapi.GeneratedCard{
			Id:         mustParseUUID(card.ID),
			AssetId:    mustParseUUID(card.AssetID),
			CardTypeId: card.CardTypeID,
			PreviewUrl: card.PreviewURL,
		}
	}
	p.Cards = cards

	return p
}

func toProjectContracts(list []projects.Project) []openapi.Project {
	result := make([]openapi.Project, len(list))
	for i, project := range list {
		result[i] = toProjectContract(project)
	}

	return result
}
