APP_NAME=kartochki-online-backend

.PHONY: dev
dev:
	go run ./cmd/api

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: infra-up
infra-up:
	docker compose up -d

.PHONY: infra-down
infra-down:
	docker compose down

