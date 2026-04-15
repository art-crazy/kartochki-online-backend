# Project Guide For Coding Agents

This repository contains the backend for `kartochki.online`, a SaaS product for marketplace sellers.

## Read This First

- Use this file as the high-signal entry point.
- Use `docs/architecture.md` for deeper architectural context.
- Keep changes production-ready. Avoid shortcuts that create migration debt.

## Documentation And Commenting Rules

- This repository is maintained by a developer who is new to Go, so code must stay readable and educational.
- Write comments and documentation in simple Russian.
- Every new package, exported type, exported function, exported method, exported interface, and exported constant group should have a Go doc comment in Russian.
- Add short Russian comments for non-obvious business rules, data flow, integration behavior, retry logic, SQL intent, and background job behavior.
- When changing existing code, improve nearby comments if they are missing, outdated, too vague, or written in a way that is hard for a beginner to understand.
- Do not comment every line. Comments should explain intent and reasoning, not restate obvious syntax.
- Prefer comments that answer one of these questions:
  - what this code is responsible for
  - why this branch or check exists
  - what assumptions the code relies on
  - what side effects happen here
  - what can break if this logic changes
- Keep comments aligned with the code. If code changes, update or remove stale comments in the same patch.
- When introducing a new subsystem or package, include a short package-level description in Russian if it helps explain the role of that package.

### Comment Style

- Keep wording direct, simple, and beginner-friendly.
- Prefer complete short sentences over shorthand.
- Avoid decorative comments and obvious comments like "увеличиваем i на 1".
- For doc comments, start with the symbol name when it fits Go conventions.
- If a function is important but the implementation is short, still document what comes in, what comes out, and what side effect matters.

### Examples

- Good: `// CreateProject создаёт проект и подготавливает запись для фоновой генерации.`
- Good: `// Повторная отправка безопасна, потому что задача идемпотентно обновляет статус по external_id.`
- Bad: `// Цикл по массиву`
- Bad: `// Устанавливаем значение переменной`

### Layer-Specific Expectations

- HTTP handlers:
  - Add a short comment when the request flow is not obvious from the function name alone.
  - Explain important validation, authorization, response mapping, and why a request is rejected.
- Application services and use cases:
  - Document the business scenario the code orchestrates.
  - Explain cross-package coordination, side effects, and why the order of steps matters.
- Repositories and persistence code:
  - Comment business-critical query intent when the SQL or method name is not self-explanatory.
  - Explain locking, transactional assumptions, uniqueness guarantees, and idempotency safeguards.
- Background jobs:
  - Document payload meaning, retry expectations, idempotency strategy, and external side effects.
  - Explain what makes the job safe or unsafe to run more than once.
- Infrastructure adapters:
  - Comment non-obvious provider behavior, fallback logic, timeout assumptions, and error translation.

### Completion Checklist

- Before finishing a task, quickly verify that:
  - new exported symbols have Russian Go doc comments
  - non-obvious logic has short Russian explanation comments nearby
  - stale comments were updated or removed together with the code
  - comments explain intent, not syntax

## Product Context

- Primary domain: `kartochki-online.ru`
- Brand name: `kartochki.online`
- The product helps users generate product-card visuals and content assets for marketplace listings.
- The frontend is a separate `Next.js` application and will auto-generate API clients from the backend OpenAPI spec.

## Stack

- Go
- chi
- PostgreSQL
- pgx
- sqlc
- Redis
- Asynq
- zerolog
- OpenAPI-first API contract

## Architecture Rules

- Keep the project a modular monolith unless there is a concrete operational reason to split services.
- Treat `api/openapi/openapi.yaml` as the external contract source of truth.
- Keep HTTP transport, domain logic, persistence, and background jobs separated.
- Prefer explicit packages over generic shared helpers.
- Keep handwritten files at 300 lines or less. If a file grows beyond 300 lines, decompose it by responsibility before adding more logic.
- Avoid hidden magic and code generation that obscures business rules, except where it gives clear mechanical value such as `sqlc`.
- Use background jobs for long-running and retryable work instead of stretching request/response handlers.

## Layering Rules

- `cmd`
  - application entrypoints only
- `internal/http`
  - routing, handlers, middleware, request and response models
- `internal/app`
  - application wiring and dependency assembly
- `internal/platform`
  - infrastructure adapters such as PostgreSQL, Redis, storage, and external providers
- `internal/jobs`
  - Asynq task definitions, enqueueing, and workers
- `internal/...` domain packages to be added later
  - business use cases, services, repositories, and entity logic
- `api/openapi`
  - public API contract for frontend generation
- `db/migrations`
  - SQL migrations
- `db/queries`
  - SQL source files for `sqlc`

## API Contract Rules

- Update `api/openapi/openapi.yaml` whenever public API behavior changes.
- Keep operation IDs stable because frontend client generation depends on them.
- Do not expose undocumented endpoints that frontend code will rely on.
- Prefer additive API changes; avoid breaking response and request shapes without an explicit migration plan.
- Keep error envelopes consistent across endpoints.

## Database Rules

- PostgreSQL is the primary system of record.
- Write schema changes as forward migrations in `db/migrations`.
- Keep SQL explicit and reviewable.
- Prefer `sqlc`-generated typed queries over handwritten generic repository layers.
- Avoid introducing an ORM unless there is a strong reason.

## Background Job Rules

- Use Asynq for retryable background work.
- Keep handlers idempotent where possible.
- Put slow generation, exports, notifications, and external syncs into jobs instead of synchronous HTTP handlers.
- Model job payloads explicitly.

## Domain Direction

The backend will likely grow around these domains:

- auth and sessions
- users and workspaces
- subscriptions and billing
- projects
- product cards and templates
- generation jobs
- asset storage
- marketplace integrations

Do not prematurely introduce separate services for these domains. Keep them modular inside one codebase first.

## Commands

- `go mod tidy`
- `go run ./cmd/api`
- `make check-file-lines`
- `docker compose up -d`
- `docker compose down`

## Change Discipline

1. Identify the owning package before creating files.
2. Keep HTTP handlers thin.
3. Put orchestration in use-case or service packages, not in transport or repository code.
4. Update `api/openapi/openapi.yaml` for public API changes.
5. Add or update simple Russian comments when changing code so the patch remains understandable to a Go beginner.
6. Before finishing, run `make check-file-lines` when code or contract files changed. Existing entries in `.line-limit-ignore` are temporary decomposition debt, not examples to copy.
7. Update `docs/architecture.md` when package boundaries or backend conventions change.
