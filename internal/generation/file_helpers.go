package generation

import (
	"path/filepath"
	"strings"

	"kartochki-online-backend/internal/projects"
)

const defaultProjectTitle = "Новый проект"

func buildProjectTitle(projectName string, sourceFileName string) string {
	projectName = strings.TrimSpace(projectName)
	if projectName != "" {
		return projectName
	}

	base := strings.TrimSpace(strings.TrimSuffix(sourceFileName, filepath.Ext(sourceFileName)))
	if base == "" {
		return defaultProjectTitle
	}
	if len(base) > projects.MaxProjectTitleLength {
		return base[:projects.MaxProjectTitleLength]
	}
	return base
}

func normalizeUploadedImage(image UploadedImage) UploadedImage {
	image.FileName = strings.TrimSpace(image.FileName)
	image.ContentType = strings.TrimSpace(strings.ToLower(image.ContentType))
	return image
}

func normalizeImageType(fileName string, contentType string) (string, string, error) {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(fileName)))
	switch {
	case contentType == "image/png" || ext == ".png":
		return ".png", "image/png", nil
	case contentType == "image/jpeg" || contentType == "image/jpg" || ext == ".jpg" || ext == ".jpeg":
		return ".jpg", "image/jpeg", nil
	case contentType == "image/webp" || ext == ".webp":
		return ".webp", "image/webp", nil
	default:
		return "", "", ErrImageTypeNotSupported
	}
}

func sanitizeFileSegment(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", "_", "-")
	value = replacer.Replace(value)
	if value == "" {
		return "card"
	}
	return value
}

func trimErrorMessage(err error) string {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "generation failed"
	}
	if len(message) > 500 {
		return message[:500]
	}
	return message
}
