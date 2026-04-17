# kartochki-online-backend

Backend-сервис для `kartochki.online` на Go.

## Документация для AI-агентов

- `AGENTS.md` — основной high-signal entrypoint для coding agents
- `CLAUDE.md` — короткие инструкции для Claude-style агентов
- `docs/architecture.md` — подробная архитектурная документация

## Стек

- Go
- chi
- PostgreSQL
- pgx
- sqlc
- Redis
- Asynq
- zerolog
- OpenAPI-first (`api/openapi/openapi.yaml`)

## Что уже есть

- базовая структура проекта
- entrypoint API
- HTTP router
- health endpoints
- конфиг через env
- заготовки под PostgreSQL, Redis и Asynq
- OpenAPI-спека и endpoint для её отдачи
- `docker-compose.yml` для локального Postgres и Redis

## Старт

1. Установить Go.
2. Скопировать `.env.example` в `.env`.
3. Поднять инфраструктуру:

```bash
docker compose up -d
```

4. Скачать зависимости и запустить API:

```bash
go mod tidy
go run ./cmd/api
```

## Служебные команды

Для повседневной работы с проектом подготовлены `make`-цели:

```bash
make dev
make build
make sqlc
make migrate-up
make migrate-down
make migrate-version
make blog-sync # вручную синхронизировать content/blog в текущую БД
make bundle    # собрать api/openapi/openapi.yaml из src/
make generate  # сгенерировать Go-типы в api/gen/openapi.gen.go
make check     # проверить кодировку и лимит строк
make check-encoding  # найти невалидный UTF-8 и похожий на mojibake текст
make check-file-lines  # проверить лимит 300 строк для ручных файлов
make install-git-hooks  # подключить pre-commit проверки из .githooks/
```

`migrate-*` используют `POSTGRES_DSN` из env или значение по умолчанию из `Makefile`.

`make blog-sync` использует тот же `POSTGRES_DSN` и нужен для локальной разработки или аварийного ручного догона контента. В production синхронизация блога должна выполняться автоматически в пайплайне деплоя.

`make check-file-lines` проверяет, что ручные файлы проекта не превышают 300 строк.
Старые превышения временно перечислены в `.line-limit-ignore`; новые большие файлы лучше сразу разделять по ответственности.

`make check-encoding` проверяет текстовые файлы на невалидный UTF-8 и типичные следы mojibake вроде повреждённой кириллицы.
`make install-git-hooks` включает pre-commit hook, который запускает проверки перед коммитом.

## OpenAPI

Спека хранится в виде многофайловой структуры и бандлится в единый файл:

```
api/openapi/src/          ← исходники (редактировать здесь)
api/openapi/openapi.yaml  ← сбандленный файл (коммитить)
api/gen/openapi.gen.go    ← сгенерированные Go-типы (коммитить)
```

После изменений в `src/` нужно пересобрать и перегенерировать:

```bash
make bundle && make generate
```

Во время работы API спека отдаётся по:

- `GET /openapi/openapi.yaml`

Swagger UI доступен по:

- `GET /swagger`

Это можно использовать на фронтенде для автогенерации клиента, а в браузере для ручной проверки и просмотра документации.

## HTTP-конвенции

- Каждый JSON-ответ пробрасывает `X-Request-ID`.
- Transport-ошибки должны возвращаться в едином виде: `code`, `message`, `request_id`, `details`.
- Один и тот же `request_id` должен совпадать в ответе и в логах запроса.
