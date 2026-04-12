# Architecture

## Purpose

This repository is the backend for `kartochki.online`, a SaaS product for marketplace sellers.

It should support two distinct but related needs:

- a stable application backend for the `Next.js` frontend
- asynchronous generation and asset-processing workflows that do not belong in synchronous HTTP requests

The architecture should optimize for predictable growth, clear request flow, strong API contracts, and operational simplicity.

## Core Stack

- `Go`
- `chi`
- `PostgreSQL`
- `pgx`
- `sqlc`
- `Redis`
- `Asynq`
- `zerolog`
- `OpenAPI`

## Code Documentation Standard

This project is developed with an educational requirement: the code should be understandable to a developer who is new to Go.

Because of that, code documentation is part of the production standard, not an optional cleanup task.

### Primary rule

Write code comments and package documentation in simple Russian.

The goal of comments is not to translate Go syntax. The goal is to explain responsibility, intent, important decisions, and side effects in a way that helps a beginner read the code confidently.

### What must be documented

Document in Russian:

- new packages when a short package description helps explain ownership
- exported structs, interfaces, type aliases, constants, and variables when they define meaningful behavior
- exported functions and methods
- non-obvious branches in handlers, services, repositories, and jobs
- retry behavior, idempotency guarantees, and external integration assumptions
- SQL queries or persistence code when business intent is not obvious from names alone

### What should usually not be documented

Avoid comments that only restate the code mechanically, for example:

- "цикл по массиву"
- "присваиваем значение переменной"
- "проверяем ошибку"

Such comments create noise and make real explanations harder to notice.

### Preferred comment style

Comments should be:

- short and direct
- written in simple language without unnecessary jargon
- focused on why the code exists or what responsibility it has
- updated together with the code in the same change

For Go doc comments, prefer the standard style where the comment starts with the exported name when it fits naturally.

Examples:

- `// ProjectService управляет сценариями работы с проектами и не знает о деталях HTTP.`
- `// Повторный запуск безопасен: мы ищем запись по external_id и не создаём дубликат.`
- `// Неудачный пример: смешение русского комментария и случайного английского текста.`

The last style should be avoided unless there is a very strong reason to keep original terminology.

### Layer-oriented guidance

Use comments where they provide the most learning value:

- in handlers: explain non-obvious validation, authorization, and response decisions
- in services: explain the business scenario and important orchestration steps
- in persistence code: explain business intent, locking, transaction boundaries, and idempotency assumptions
- in jobs: explain payload meaning, retry safety, and repeated-run behavior
- in infrastructure adapters: explain provider-specific behavior, timeout assumptions, and error translation

## Why This Stack

### Why Go

Go is a strong fit for this backend because it gives:

- simple deployment
- good concurrency for jobs and integrations
- predictable runtime behavior
- clear, explicit code for long-lived backend systems

This project is not just CRUD. It will likely include generation pipelines, file handling, retries, integration callbacks, and scheduled or queued work. Go fits that shape well.

### Why PostgreSQL

PostgreSQL should be the system of record for:

- users
- workspaces
- subscriptions
- projects
- templates
- generation jobs
- audit and billing-related records

The domain is relational, and correctness matters more than schema flexibility.

### Why `pgx` + `sqlc`

This combination keeps SQL explicit while preserving typed access in Go.

Benefits:

- queries stay readable and reviewable
- generated types reduce boilerplate and runtime mapping errors
- schema changes stay close to query changes
- there is less hidden ORM behavior to debug

### Why Redis + Asynq

The product will likely need background execution for:

- image generation
- export creation
- notification sending
- webhook retries
- third-party synchronization
- expensive cleanup or maintenance tasks

These concerns should not live in synchronous API handlers.

## Architectural Style

The backend should remain a modular monolith by default.

That means:

- one deployable API application at the start
- one primary PostgreSQL database
- one Redis instance for queues and ephemeral coordination
- internal package boundaries for domains instead of early microservices

This is the right default because the product domain is still evolving. Splitting into services too early would increase operational cost and slow down refactors.

## Request and Execution Model

### Synchronous path

Use HTTP requests for:

- authentication
- dashboard and workspace reads
- CRUD-like product interactions
- idempotent control-plane actions
- enqueueing asynchronous work

Handlers should:

- validate input
- call a use-case or application service
- map results into transport responses
- avoid embedding business orchestration directly

### Asynchronous path

Use Asynq tasks for:

- long-running generation
- retries against external systems
- heavy asset processing
- outbound communication that does not need to block the user

Jobs should:

- have explicit payload types
- be safe to retry
- emit structured logs with job identifiers
- avoid hidden side effects

## OpenAPI-First Contract

The frontend will generate an API client from Swagger/OpenAPI, so the contract discipline matters.

### Source of truth

The canonical public contract lives in:

- `api/openapi/openapi.yaml`

The running API should expose this file so tooling can consume it directly.

### Rules

- keep operation IDs stable
- prefer additive changes
- document request and response schemas before frontend integration depends on them
- maintain a consistent error model
- do not create undocumented frontend-critical endpoints

If the implementation and the spec diverge, the spec is no longer useful. That must be treated as a defect.

## Suggested Package Structure

The current skeleton is intentionally thin. As the project grows, use the structure below.

### `cmd/api`

Application entrypoint only.

Responsibilities:

- load config
- initialize logger
- assemble dependencies
- start and stop the HTTP server

Do not place domain logic here.

### `internal/app`

Composition root.

Responsibilities:

- wire infrastructure
- build services and handlers
- own application startup and shutdown assembly

This package should explain how the system is connected, not contain business logic.

### `internal/http`

Transport layer.

Responsibilities:

- router setup
- middleware
- handlers
- transport DTOs
- request parsing and response mapping

Transport DTOs may live in explicit subpackages when that keeps contracts readable, for example:

- `internal/http/contracts`

Transport response helpers may live in a small dedicated package when that keeps handlers thin and preserves one error format for the whole API.

Do not put database access or cross-domain orchestration directly in handlers.

### `internal/platform`

Infrastructure adapters.

Examples:

- PostgreSQL connection setup (`internal/platform/postgres`)
- Redis setup (`internal/platform/redis`)
- Email adapters — реализации интерфейса `auth.EmailSender` (`internal/platform/email`)
- Local file storage for uploaded source images and generated artifacts (`internal/platform/storage`)
- S3 adapter
- payment provider clients

Keep these packages focused on external systems and low-level integration details.

### `internal/jobs`

Background execution boundary.

Responsibilities:

- task names and payloads
- enqueueing
- workers and handlers
- job middleware if needed later

### Adapter pattern for domain-to-jobs wiring

Domain packages (`internal/auth`, `internal/generation`, etc.) must not import `internal/jobs` directly to avoid circular dependencies.

Instead, use one of two approaches:

1. **Domain implements a jobs interface** — when the domain service needs to be called *by* a worker. The domain method signature matches the `jobs.*Handler` interface, and `internal/app` passes the domain service directly (see `generation.Service` implementing `jobs.GenerationHandler`).

2. **App-level adapter** — when wiring would create a cycle. `internal/app` defines a small adapter struct that holds only what the worker needs (e.g. `auth.EmailSender` + timeout), implements the `jobs.*Handler` interface, and is passed to `jobs.NewServer`. The domain package never sees `jobs`. (see `authEmailWorker` in `internal/app/app.go`).

A `var _ jobs.XHandler = adapterType{}` compile-time check must accompany every adapter.

### Future domain packages

As business logic appears, add explicit domain-owned packages such as:

- `internal/auth`
- `internal/settings`
- `internal/users`
- `internal/workspaces`
- `internal/projects`
- `internal/templates`
- `internal/generation`
- `internal/billing`
- `internal/blog`
- `internal/integrations`

Each domain can contain its own:

- entities or models
- service or use-case logic
- repository interfaces
- transport mapping helpers if truly domain-specific

Keep ownership explicit. Avoid giant cross-domain utility packages.

`internal/settings` is now used for the `/api/v1/settings` surface.

It owns:

- profile editing beyond core auth fields
- generation defaults
- notification preferences
- active session management
- API key rotation
- account export enqueueing
- account deletion orchestration

Security-sensitive primitives such as password hashing and bearer-session authentication still remain in `internal/auth`.

`internal/generation` is now used for the `/api/v1/generate/*`, `/api/v1/uploads/images`,
and `/api/v1/generations/*` surface.

It owns:

- generation page config
- source image upload orchestration
- creation of generation jobs and linked projects
- polling-ready generation status reads
- coordination with Asynq worker and local artifact storage

`internal/blog` is now used for the public `/api/v1/public/blog` and `/api/v1/public/blog/{slug}` surface.

It owns:

- read-only loading of published SEO articles
- server pagination for the public blog list
- mapping typed article sections from JSON payloads
- related posts, tags, categories, and sidebar data for blog pages

## Data and Persistence Conventions

### Migrations

Use forward SQL migrations in:

- `db/migrations`

Rules:

- one logical schema change per migration
- do not rewrite old migrations after they are shared
- keep destructive changes deliberate and reversible where practical

### Queries

Use SQL files in:

- `db/queries`

Generate typed access via `sqlc`.

This keeps persistence explicit and avoids opaque repository frameworks.

## Domain Modeling Direction

The likely early aggregates and records are:

- `User`
- `Workspace`
- `Membership`
- `Subscription`
- `Project`
- `Template`
- `GenerationJob`
- `Asset`
- `MarketplaceAccount`

Not all of these need to exist immediately. Add them when product requirements demand them, but preserve coherent naming and ownership as they appear.

## API Conventions

### Versioning

Expose public API endpoints under a versioned prefix such as:

- `/api/v1`

This is not implemented yet in the skeleton, but it should be the default direction for real endpoints.

### Error model

Use one consistent envelope for application errors.

The project should eventually standardize fields similar to:

- machine-readable code
- human-readable message
- request ID
- optional field-level details

This matters because generated frontend clients and UI flows become more predictable when error shapes are stable.

At the transport level it is reasonable to converge on:

- `code`
- `message`
- `request_id`
- `details`

`request_id` should also be returned in the `X-Request-ID` header so frontend logs, browser inspection, and backend logs refer to the same identifier.

### Health endpoints

Keep simple infrastructure endpoints separate from business routes:

- `/health/live`
- `/health/ready`

These are already part of the skeleton.

## Logging and Observability

Use structured logs everywhere.

Prefer including:

- request ID
- user ID when available
- job ID when available
- domain identifiers such as workspace ID or project ID

For HTTP, it is useful to build a request-scoped logger in middleware and pass it through context. This keeps handler logs and access logs aligned on the same request metadata without repeating field assembly in every handler.

As the backend grows, add:

- Prometheus metrics
- OpenTelemetry tracing
- Sentry or equivalent error tracking

Observability should be additive and boring, not framework-heavy.

## Config Rules

Use environment variables as the base configuration mechanism.

Rules:

- keep defaults safe for local development
- avoid hardcoding secrets
- group config by subsystem
- parse values into typed config structs at startup

The current skeleton already follows this direction.

## What Not To Do Early

- do not split into microservices
- do not introduce a heavy ORM
- do not put business rules into HTTP handlers
- do not let queue payloads become undocumented blobs
- do not let OpenAPI lag behind implementation
- do not create giant `utils` packages with mixed ownership

## Near-Term Evolution Path

The next reasonable steps for this backend are:

1. Add `/api/v1` route grouping.
2. Define a standard error envelope and response helpers.
3. Add migration tooling and first schema files.
4. Create initial domains:
   - auth
   - users
   - projects
   - generation
5. Add job payload definitions and worker process entrypoints.
6. Expand OpenAPI alongside each public endpoint.

## Documentation Rule

When package boundaries, transport conventions, persistence strategy, comment conventions, or contract rules change, update this file.
