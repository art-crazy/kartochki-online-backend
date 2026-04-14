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

### File size limit

Handwritten files should stay at 300 lines or less.

When a file grows beyond this limit, split it by responsibility instead of adding more branches, helpers, or unrelated behavior to the same file. Good split points are transport mapping, validation, use-case orchestration, provider adapters, repository queries, and background job handlers.

Generated files and bundled artifacts are exempt from this rule because they are not edited by hand. Existing handwritten files listed in `.line-limit-ignore` are temporary decomposition debt. When touching one of them, prefer extracting a focused file and removing the path from the baseline.

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

The spec is authored as a multi-file source tree and bundled into a single file for tooling:

- `api/openapi/src/openapi.yaml` — root source file, references all path and schema fragments
- `api/openapi/src/paths/` — one YAML file per route group
- `api/openapi/src/components/schemas/` — one YAML file per domain
- `api/openapi/openapi.yaml` — bundled output, committed to the repo, consumed by `embed.FS` and `oapi-codegen`

### Code generation pipeline

1. Edit the source files in `api/openapi/src/`.
2. Run `make bundle` — uses `@redocly/cli` to produce `api/openapi/openapi.yaml`.
3. Run `make generate` — uses `oapi-codegen v2` to produce `api/gen/openapi.gen.go` (package `openapi`, models only).
4. Commit both the bundled YAML and the generated Go file.

The generated Go types in `api/gen` are the single authoritative set of request and response types for the HTTP layer. Handlers must not define parallel struct types for the same purpose.

`api/openapi/oapi-codegen.yaml` controls generation: models only, no server stubs, no client code.

### Rules

- keep operation IDs stable
- prefer additive changes
- document request and response schemas before frontend integration depends on them
- maintain a consistent error model
- do not create undocumented frontend-critical endpoints
- after any spec edit, re-run `make bundle && make generate` before committing

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
- request parsing and response mapping

Transport DTOs are generated from the OpenAPI spec and live in `api/gen` (package `openapi`). Handlers import that package directly — there is no separate `internal/http/contracts` package.

Transport response helpers live in `internal/http/response`. The `WriteError` function accepts `openapi.ErrorDetail` from `api/gen` and builds the consistent error envelope from there.

Shared handler utilities (UUID parsing, JSON decoding, auth context extraction) live in `internal/http/handlers/helpers.go` as package-level functions so that individual handler structs stay focused on their own domain.

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

Instead, use app-level adapters:

`internal/app` defines a small adapter struct that holds only what the worker needs, implements the `jobs.*Handler` interface, and is passed to `jobs.NewServer`. The domain package never sees `jobs`.

Examples:

- `authEmailWorker` adapts `auth.EmailSender` to `jobs.SendPasswordResetEmailHandler`.
- `generationWorkerAdapter` adapts `generation.Service` to `jobs.GenerationHandler`.
- `asynqGenerationEnqueuer` adapts `jobs.Client` to `generation.GenerationJobEnqueuer`.

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

- generation page config (marketplaces, styles, card types, card count options, AI model catalog)
- source image upload orchestration
- creation of generation jobs and linked projects
- polling-ready generation status reads
- coordination with Asynq worker and local artifact storage
- AI image generation via the `ImageGenerator` interface

The `ImageGenerator` interface is defined in `internal/generation` and implemented in `internal/platform/routerai`. Wiring lives in `internal/app` via `routerAIAdapter` — the same adapter pattern used for `authEmailWorker` and `yookassaCheckoutAdapter`.

Generation enqueueing uses `generation.GenerationJobEnqueuer`; `internal/app` adapts the concrete `jobs.Client` via `asynqGenerationEnqueuer`. Worker handling also goes through `generationWorkerAdapter`, so `internal/generation` does not import `internal/jobs`.

When `ROUTERAI_API_KEY` is not set, `generation.NewService` receives `nil` and substitutes a `noopImageGenerator` that returns an error, causing the generation job to fail visibly rather than silently produce empty files.

`internal/blog` is now used for the public `/api/v1/public/blog` and `/api/v1/public/blog/{slug}` surface.

It owns:

- read-only loading of published SEO articles
- server pagination for the public blog list
- mapping typed article sections from JSON payloads
- related posts, tags, categories, and sidebar data for blog pages

`internal/billing` is now used for the `/api/v1/billing/*` surface.

It owns:

- subscription and usage quota state
- checkout session creation (delegated to a `CheckoutProvider` interface)
- webhook event handling (idempotent, transactional)
- generation quota enforcement via `EnsureGenerationAllowed`

The `CheckoutProvider` interface is defined in `internal/billing` and implemented in `internal/platform/yookassa`. Wiring lives in `internal/app` via `yookassaCheckoutAdapter` to keep the domain and platform packages decoupled (same adapter pattern as `authEmailWorker`).

`internal/platform/yookassa` implements the ЮКасса HTTP API:

- payment creation for subscriptions and addons
- HMAC-SHA256 webhook signature verification
- deterministic idempotency keys (SHA256 of input params + year-month) to prevent duplicate payments on retry while allowing new payments in subsequent periods

`internal/platform/routerai` implements the RouterAI image generation API:

- sends requests to `/api/v1/chat/completions` with `modalities: ["image", "text"]`
- decodes the base64 PNG from `choices[0].message.images[0].image_url.url`
- model is passed per-request (not stored on the client) so the user can choose a model per generation
- configured via `ROUTERAI_API_KEY`, `ROUTERAI_ENDPOINT`, `ROUTERAI_TIMEOUT`

### Webhook handler pattern

Incoming payment provider webhooks follow this flow:

1. `BillingWebhookHandler` (transport) reads and signature-verifies the raw body
2. `yookassaEventToBilling` converts the provider-specific event to a domain `billing.WebhookEvent` (strings normalized at this boundary)
3. `billing.Service.HandleWebhookEvent` handles the event in a single DB transaction
4. Idempotency is enforced by `MarkPaymentPaid WHERE status = 'pending'` — `affected == 0` means the webhook was already processed, no activation occurs
5. On any error, the handler returns 500 so the provider retries

The webhook endpoint does not require user authentication; it is protected by signature verification only.

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
