APP_NAME=kartochki-online-backend
POSTGRES_DSN?=postgres://postgres:postgres@localhost:5432/kartochki_online?sslmode=disable
SQLC_VERSION?=v1.29.0
MIGRATE_VERSION?=v4.18.3
OAPI_CODEGEN_VERSION?=v2.4.1

OPENAPI_SRC=api/openapi/src/openapi.yaml
OPENAPI_BUNDLE=api/openapi/openapi.yaml
OPENAPI_GEN=api/gen/openapi.gen.go
OPENAPI_CFG=api/openapi/oapi-codegen.yaml

# bundle: собирает многофайловую src/ спецификацию в единый openapi.yaml для embed и oapi-codegen.
# Требует npx (Node.js). Результат коммитится в репозиторий.
.PHONY: bundle
bundle:
	npx --yes @redocly/cli@latest bundle $(OPENAPI_SRC) --output $(OPENAPI_BUNDLE) --ext yaml

# generate: генерирует Go-типы из bundled спецификации.
# Запускать после bundle или при изменении openapi.yaml.
.PHONY: generate
generate:
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION) \
		--config $(OPENAPI_CFG) $(OPENAPI_BUNDLE)

.PHONY: dev
dev:
	go run ./cmd/api

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: build
build:
	go build ./...

.PHONY: blog-sync
blog-sync:
	go run ./cmd/blogsync

.PHONY: check-file-lines
check-file-lines:
	go run ./tools/check-file-lines

.PHONY: check-encoding
check-encoding:
	go run ./tools/check-encoding

.PHONY: check-staged-encoding
check-staged-encoding:
	go run ./tools/check-encoding --staged

.PHONY: check
check: check-encoding check-file-lines

.PHONY: install-git-hooks
install-git-hooks:
	git config core.hooksPath .githooks

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
