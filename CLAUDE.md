# Claude Project Instructions

This file is intended for Claude-style coding agents working in this repository.

## Start Here

- Read `AGENTS.md` first.
- Use `docs/architecture.md` for deeper rationale and package guidance.

## Mission

Build and maintain the backend for `kartochki.online`.

## Non-Negotiables

- Keep the backend a modular monolith by default.
- Treat `api/openapi/openapi.yaml` as the public API contract source of truth.
- Keep handlers thin and move orchestration into domain-level packages.
- Use PostgreSQL as the primary store.
- Use explicit SQL and `sqlc` for typed query generation.
- Use Asynq for retryable background work.
- Avoid hidden abstractions that make request flow harder to trace.

## Implementation Defaults

- REST API
- OpenAPI-first
- explicit config via env
- structured logging
- explicit infrastructure adapters
- additive schema and API evolution

## Update Rule

If your changes affect package boundaries, transport conventions, job execution, or public API contracts, update `docs/architecture.md`.

