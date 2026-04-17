package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"kartochki-online-backend/internal/blogsync"
	"kartochki-online-backend/internal/config"
	"kartochki-online-backend/internal/platform/postgres"
)

const defaultContentDir = "content/blog"

// main запускает явную синхронизацию blog YAML-файлов в PostgreSQL.
func main() {
	contentDir := flag.String("dir", defaultContentDir, "путь до каталога со статьями блога в YAML")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fail(err)
	}

	documents, err := blogsync.LoadDocuments(*contentDir)
	if err != nil {
		fail(err)
	}

	postgresClient, err := postgres.New(cfg.Postgres.DSN)
	if err != nil {
		fail(err)
	}
	defer postgresClient.Close()

	if err := blogsync.Sync(context.Background(), postgresClient.Pool, documents); err != nil {
		fail(err)
	}

	fmt.Printf("blog sync completed: %d articles\n", len(documents))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
