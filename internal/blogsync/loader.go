package blogsync

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadDocuments читает все YAML-статьи из каталога и проверяет их базовую целостность.
func LoadDocuments(dir string) ([]Document, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read blog content dir: %w", err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		paths = append(paths, filepath.Join(dir, entry.Name()))
	}

	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("no blog yaml files found in %s", dir)
	}

	documents := make([]Document, 0, len(paths))
	seenSlugs := make(map[string]string, len(paths))
	for _, path := range paths {
		document, err := loadDocument(path)
		if err != nil {
			return nil, err
		}

		if previousPath, exists := seenSlugs[document.Slug]; exists {
			return nil, fmt.Errorf("duplicate blog slug %q in %s and %s", document.Slug, previousPath, path)
		}

		seenSlugs[document.Slug] = path
		documents = append(documents, document)
	}

	return documents, nil
}

func loadDocument(path string) (Document, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Document{}, fmt.Errorf("read blog yaml %s: %w", path, err)
	}

	var document Document
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return Document{}, fmt.Errorf("decode blog yaml %s: %w", path, err)
	}

	document.SourcePath = path
	document.Slug = strings.TrimSpace(document.Slug)
	document.Title = strings.TrimSpace(document.Title)
	document.Excerpt = strings.TrimSpace(document.Excerpt)
	document.SEOTitle = strings.TrimSpace(document.SEOTitle)
	document.SEODescription = strings.TrimSpace(document.SEODescription)
	document.Status = strings.TrimSpace(document.Status)

	if err := document.Validate(); err != nil {
		return Document{}, err
	}

	return document, nil
}
