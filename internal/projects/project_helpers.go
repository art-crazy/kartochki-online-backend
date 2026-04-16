package projects

import (
	"strings"

	"kartochki-online-backend/internal/dbgen"
)

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
