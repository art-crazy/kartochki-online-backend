# Project Guide For Coding Agents

This repository contains the backend for `kartochki.online`, a SaaS product for marketplace sellers.

## Read This First

- Use this file as the high-signal entry point.
- Use `docs/architecture.md` for deeper architectural context.
- Keep changes production-ready. Avoid shortcuts that create migration debt.

## Product Context

- Primary domains:
  - `kartochki-online.ru`
  - `xn----7sbabjowfpen9ag6h.xn--p1ai`
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
- `docker compose up -d`
- `docker compose down`

## Change Discipline

1. Identify the owning package before creating files.
2. Keep HTTP handlers thin.
3. Put orchestration in use-case or service packages, not in transport or repository code.
4. Update `api/openapi/openapi.yaml` for public API changes.
5. Update `docs/architecture.md` when package boundaries or backend conventions change.

