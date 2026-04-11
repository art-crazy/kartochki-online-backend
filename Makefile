APP_NAME=kartochki-online-backend
POSTGRES_DSN?=postgres://postgres:postgres@localhost:5432/kartochki_online?sslmode=disable
SQLC_VERSION?=v1.29.0
MIGRATE_VERSION?=v4.18.3

.PHONY: dev
dev:
	go run ./cmd/api

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: build
build:
	go build ./...

.PHONY: sqlc
sqlc:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

.PHONY: migrate-up
migrate-up:
	go run -tags=postgres github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION) -path db/migrations -database "$(POSTGRES_DSN)" up

.PHONY: migrate-down
migrate-down:
	go run -tags=postgres github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION) -path db/migrations -database "$(POSTGRES_DSN)" down 1

.PHONY: migrate-version
migrate-version:
	go run -tags=postgres github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION) -path db/migrations -database "$(POSTGRES_DSN)" version

.PHONY: infra-up
infra-up:
	docker compose up -d

.PHONY: infra-down
infra-down:
	docker compose down
