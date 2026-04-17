package blogsync

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const (
	statusDraft     = "draft"
	statusPublished = "published"
)

var allowedSectionKinds = map[string]struct{}{
	"text":    {},
	"list":    {},
	"table":   {},
	"callout": {},
	"cards":   {},
	"steps":   {},
}

// Document описывает одну статью блога в YAML-файле.
type Document struct {
	Slug           string        `yaml:"slug"`
	Title          string        `yaml:"title"`
	Excerpt        string        `yaml:"excerpt"`
	SEOTitle       string        `yaml:"seo_title"`
	SEODescription string        `yaml:"seo_description"`
	Status         string        `yaml:"status"`
	PublishedAt    *time.Time    `yaml:"published_at"`
	Categories     []TaxonomyRef `yaml:"categories"`
	Tags           []TaxonomyRef `yaml:"tags"`
	Metrics        Metrics       `yaml:"metrics"`
	Sections       []Section     `yaml:"sections"`
	SourcePath     string        `yaml:"-"`
}

// TaxonomyRef описывает категорию или тег статьи.
type TaxonomyRef struct {
	Slug  string `yaml:"slug"`
	Label string `yaml:"label"`
}

// Metrics хранит числовые значения для sidebar и SEO-блоков.
type Metrics struct {
	ViewsCount         int `yaml:"views_count"`
	ReadingTimeMinutes int `yaml:"reading_time_minutes"`
}

// Section описывает один блок статьи.
type Section struct {
	Kind    string `yaml:"kind"`
	Title   string `yaml:"title"`
	Level   int    `yaml:"level"`
	Payload any    `yaml:"payload"`
}

// Validate проверяет, что YAML-документ можно безопасно синхронизировать в БД.
func (d Document) Validate() error {
	if strings.TrimSpace(d.Slug) == "" {
		return fmt.Errorf("%s: slug must not be empty", filepath.Base(d.SourcePath))
	}
	if strings.TrimSpace(d.Title) == "" {
		return fmt.Errorf("%s: title must not be empty", filepath.Base(d.SourcePath))
	}
	if d.Status != statusDraft && d.Status != statusPublished {
		return fmt.Errorf("%s: unsupported status %q", filepath.Base(d.SourcePath), d.Status)
	}
	if d.Status == statusPublished && d.PublishedAt == nil {
		return fmt.Errorf("%s: published article must define published_at", filepath.Base(d.SourcePath))
	}
	if d.Metrics.ReadingTimeMinutes <= 0 {
		return fmt.Errorf("%s: reading_time_minutes must be greater than zero", filepath.Base(d.SourcePath))
	}
	if d.Metrics.ViewsCount < 0 {
		return fmt.Errorf("%s: views_count must not be negative", filepath.Base(d.SourcePath))
	}
	if len(d.Sections) == 0 {
		return fmt.Errorf("%s: article must contain at least one section", filepath.Base(d.SourcePath))
	}

	for _, item := range d.Categories {
		if err := validateTaxonomyRef("category", item, d.SourcePath); err != nil {
			return err
		}
	}
	for _, item := range d.Tags {
		if err := validateTaxonomyRef("tag", item, d.SourcePath); err != nil {
			return err
		}
	}
	for index, section := range d.Sections {
		if err := validateSection(index, section, d.SourcePath); err != nil {
			return err
		}
	}

	return nil
}

func validateTaxonomyRef(kind string, item TaxonomyRef, sourcePath string) error {
	if strings.TrimSpace(item.Slug) == "" {
		return fmt.Errorf("%s: %s slug must not be empty", filepath.Base(sourcePath), kind)
	}
	if strings.TrimSpace(item.Label) == "" {
		return fmt.Errorf("%s: %s label must not be empty", filepath.Base(sourcePath), kind)
	}

	return nil
}

func validateSection(index int, section Section, sourcePath string) error {
	kind := strings.TrimSpace(section.Kind)
	if _, ok := allowedSectionKinds[kind]; !ok {
		return fmt.Errorf("%s: unsupported section kind %q at index %d", filepath.Base(sourcePath), section.Kind, index)
	}
	if section.Payload == nil {
		return fmt.Errorf("%s: section payload must not be nil at index %d", filepath.Base(sourcePath), index)
	}
	if section.Level != 0 && (section.Level < 1 || section.Level > 6) {
		return fmt.Errorf("%s: section level must be between 1 and 6 at index %d", filepath.Base(sourcePath), index)
	}
	if err := validateSectionPayload(kind, section.Payload, sourcePath, index); err != nil {
		return err
	}

	return nil
}

func validateSectionPayload(kind string, payload any, sourcePath string, index int) error {
	sourceName := filepath.Base(sourcePath)
	payloadMap, ok := payload.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: section payload must be an object for kind %q at index %d", sourceName, kind, index)
	}

	switch kind {
	case "text":
		if !hasNonEmptyString(payloadMap, "body") {
			return fmt.Errorf("%s: text section must contain non-empty body at index %d", sourceName, index)
		}
	case "list":
		if !hasStringList(payloadMap, "items") {
			return fmt.Errorf("%s: list section must contain non-empty items at index %d", sourceName, index)
		}
	case "table":
		if !hasStringList(payloadMap, "head") || !hasStringMatrix(payloadMap, "rows") {
			return fmt.Errorf("%s: table section must contain head and rows at index %d", sourceName, index)
		}
	case "callout":
		if !hasNonEmptyString(payloadMap, "tone") || !hasNonEmptyString(payloadMap, "title") || !hasNonEmptyString(payloadMap, "text") {
			return fmt.Errorf("%s: callout section must contain tone, title and text at index %d", sourceName, index)
		}
	case "cards":
		if !hasObjectList(payloadMap, "cards") {
			return fmt.Errorf("%s: cards section must contain non-empty cards at index %d", sourceName, index)
		}
	case "steps":
		if !hasObjectList(payloadMap, "steps") {
			return fmt.Errorf("%s: steps section must contain non-empty steps at index %d", sourceName, index)
		}
	}

	return nil
}

func hasNonEmptyString(payload map[string]any, key string) bool {
	value, ok := payload[key]
	if !ok {
		return false
	}

	text, ok := value.(string)
	return ok && strings.TrimSpace(text) != ""
}

func hasStringList(payload map[string]any, key string) bool {
	value, ok := payload[key]
	if !ok {
		return false
	}

	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return false
	}

	for _, item := range items {
		text, ok := item.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return false
		}
	}

	return true
}

func hasStringMatrix(payload map[string]any, key string) bool {
	value, ok := payload[key]
	if !ok {
		return false
	}

	rows, ok := value.([]any)
	if !ok || len(rows) == 0 {
		return false
	}

	for _, row := range rows {
		cells, ok := row.([]any)
		if !ok || len(cells) == 0 {
			return false
		}
		for _, cell := range cells {
			text, ok := cell.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return false
			}
		}
	}

	return true
}

func hasObjectList(payload map[string]any, key string) bool {
	value, ok := payload[key]
	if !ok {
		return false
	}

	items, ok := value.([]any)
	return ok && len(items) > 0
}
