package storage

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SavedFile описывает файл, который storage сохранил и может отдать по public URL.
type SavedFile struct {
	StorageKey string
	SizeBytes  int64
}

// ArchiveFile описывает один файл, который нужно включить в zip-архив.
type ArchiveFile struct {
	Name       string
	StorageKey string
}

// Client управляет локальным файловым хранилищем для исходников и артефактов генерации.
type Client struct {
	rootDir    string
	publicBase string
}

// New создаёт локальное файловое хранилище и подготавливает корневую директорию.
func New(rootDir string, publicBase string) (*Client, error) {
	rootDir = strings.TrimSpace(rootDir)
	if rootDir == "" {
		return nil, fmt.Errorf("storage root dir must not be empty")
	}

	publicBase = strings.TrimSpace(publicBase)
	if publicBase == "" {
		publicBase = "/media"
	}
	if !strings.HasPrefix(publicBase, "/") {
		publicBase = "/" + publicBase
	}
	publicBase = strings.TrimRight(publicBase, "/")

	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, fmt.Errorf("create storage root dir: %w", err)
	}

	return &Client{
		rootDir:    rootDir,
		publicBase: publicBase,
	}, nil
}

// Save сохраняет новый файл по заданному storage key.
func (c *Client) Save(ctx context.Context, storageKey string, body []byte) (SavedFile, error) {
	if err := ctx.Err(); err != nil {
		return SavedFile{}, err
	}

	fullPath, err := c.fullPath(storageKey)
	if err != nil {
		return SavedFile{}, err
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return SavedFile{}, fmt.Errorf("create storage subdir: %w", err)
	}

	if err := os.WriteFile(fullPath, body, 0o644); err != nil {
		return SavedFile{}, fmt.Errorf("write file: %w", err)
	}

	return SavedFile{
		StorageKey: storageKey,
		SizeBytes:  int64(len(body)),
	}, nil
}

// Copy создаёт новый файл как копию уже сохранённого объекта.
func (c *Client) Copy(ctx context.Context, sourceKey string, targetKey string) (SavedFile, error) {
	if err := ctx.Err(); err != nil {
		return SavedFile{}, err
	}

	sourcePath, err := c.fullPath(sourceKey)
	if err != nil {
		return SavedFile{}, err
	}

	body, err := os.ReadFile(sourcePath)
	if err != nil {
		return SavedFile{}, fmt.Errorf("read source file: %w", err)
	}

	return c.Save(ctx, targetKey, body)
}

// Read читает файл из storage по storage key.
// Метод нужен worker-ам, когда исходник надо передать внешнему AI-провайдеру.
func (c *Client) Read(ctx context.Context, storageKey string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	fullPath, err := c.fullPath(storageKey)
	if err != nil {
		return nil, err
	}

	body, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read storage file: %w", err)
	}

	return body, nil
}

// CreateZIP собирает zip-архив из уже сохранённых файлов.
func (c *Client) CreateZIP(ctx context.Context, targetKey string, files []ArchiveFile) (SavedFile, error) {
	if err := ctx.Err(); err != nil {
		return SavedFile{}, err
	}

	fullPath, err := c.fullPath(targetKey)
	if err != nil {
		return SavedFile{}, err
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return SavedFile{}, fmt.Errorf("create archive subdir: %w", err)
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return SavedFile{}, fmt.Errorf("create archive file: %w", err)
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	for _, item := range files {
		if err := ctx.Err(); err != nil {
			_ = writer.Close()
			return SavedFile{}, err
		}

		sourcePath, err := c.fullPath(item.StorageKey)
		if err != nil {
			_ = writer.Close()
			return SavedFile{}, err
		}

		body, err := os.ReadFile(sourcePath)
		if err != nil {
			_ = writer.Close()
			return SavedFile{}, fmt.Errorf("read file for archive: %w", err)
		}

		entryName := strings.TrimSpace(item.Name)
		if entryName == "" {
			entryName = filepath.Base(item.StorageKey)
		}

		entryWriter, err := writer.Create(entryName)
		if err != nil {
			_ = writer.Close()
			return SavedFile{}, fmt.Errorf("create archive entry: %w", err)
		}

		if _, err := entryWriter.Write(body); err != nil {
			_ = writer.Close()
			return SavedFile{}, fmt.Errorf("write archive entry: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return SavedFile{}, fmt.Errorf("close archive writer: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		return SavedFile{}, fmt.Errorf("stat archive file: %w", err)
	}

	return SavedFile{
		StorageKey: targetKey,
		SizeBytes:  info.Size(),
	}, nil
}

// Delete удаляет файл best-effort, если после записи storage операция в БД не завершилась.
func (c *Client) Delete(ctx context.Context, storageKey string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	fullPath, err := c.fullPath(storageKey)
	if err != nil {
		return err
	}

	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete storage file: %w", err)
	}

	return nil
}

// PublicURL возвращает URL, по которому frontend может забрать файл напрямую.
func (c *Client) PublicURL(storageKey string) string {
	key := strings.TrimLeft(strings.TrimSpace(storageKey), "/")
	return c.publicBase + "/" + key
}

// RootDir возвращает корневую директорию storage для подключения FileServer.
func (c *Client) RootDir() string {
	return c.rootDir
}

func (c *Client) fullPath(storageKey string) (string, error) {
	key := strings.TrimSpace(storageKey)
	key = strings.TrimLeft(filepath.Clean(strings.ReplaceAll(key, "\\", "/")), "/")
	if key == "." || key == "" {
		return "", fmt.Errorf("storage key must not be empty")
	}

	fullPath := filepath.Join(c.rootDir, filepath.FromSlash(key))
	rootPath := filepath.Clean(c.rootDir)
	cleanPath := filepath.Clean(fullPath)
	if cleanPath != rootPath && !strings.HasPrefix(cleanPath, rootPath+string(filepath.Separator)) {
		return "", fmt.Errorf("storage key points outside storage root")
	}

	return cleanPath, nil
}
