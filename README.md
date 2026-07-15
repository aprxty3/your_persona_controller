# controller-api

Backend API (Go) for **Your Persona's** — an AI-powered psychological assessment platform.

## Stack

Go · PostgreSQL (GORM) · Redis · Asynq · Gemini API · Cloudflare R2 · Clean Architecture + DDD

## Prerequisites

- [Go 1.25+](https://go.dev/dl/)
- [Docker](https://docs.docker.com/get-docker/) & Docker Compose
- (Optional) [Air](https://github.com/air-verse/air) for hot-reload outside Docker

## Quick Start

```bash
cp .env.example .env          # configure secrets / DB credentials
make dev                       # start all services (Postgres, Redis, MinIO, Mailpit + Air hot-reload)
go run ./cmd/migrate           # run database migration
go run ./cmd/seed              # seed question bank & insight templates
```

Dev tools available at:
- **Mailpit** (email catcher): http://localhost:8025
- **MinIO Console** (S3 storage): http://localhost:9001 (`minioadmin` / `minioadmin`)

## Make Targets

Run `make help` (or just `make`) to see all available commands:

| Command | Description |
|---|---|
| `make dev` | Start dev environment (Air hot-reload + Postgres/Redis/MinIO/Mailpit) |
| `make prod` | Start production environment (detached) |
| `make stop` | Stop all Docker services |
| `make prune` | Stop + remove all containers & volumes (**DB data lost**) |
| `make logs` | Tail Docker logs (filter: `make logs s=api`) |
| `make run-api` | Run API server locally (without Docker) |
| `make run-worker` | Run Asynq worker locally |
| `make migrate` | Run database migration (GORM AutoMigrate) |
| `make seed` | Seed database with initial data |
| `make build` | Compile all binaries to `./bin/` |
| `make clean` | Remove build artifacts |
| `make wire` | Regenerate dependency injection (google/wire) |
| `make swag` | Regenerate Swagger API documentation |
| `make test` | Run all tests with race detector + coverage |
| `make lint` | Run golangci-lint |
| `make tidy` | Tidy go.mod and go.sum |

## Entrypoints (`cmd/`)

| Entrypoint | Purpose | When to modify |
|---|---|---|
| `cmd/api/` | HTTP API server (Echo) | Adding new routes/handlers |
| `cmd/worker/` | Background job worker (Asynq) | Adding new async task types |
| `cmd/migrate/` | Database schema migration (GORM AutoMigrate) | Adding new tables/models |
| `cmd/seed/` | Seed data (questions, templates, insight_templates) | Adding/updating seed content |

## Dependency Injection (Wire)

This project uses **Google Wire** for *compile-time dependency injection*.
- **`wire.go`**: Contains injection configuration (`//go:build wireinject`).
- **`wire_gen.go`**: Auto-generated. **Do not modify manually.**

After adding a new repository, use case, or handler constructor:
1. Register it in `wire.Build` within `cmd/api/wire.go` or `cmd/worker/wire.go`.
2. Run `make wire` to regenerate.

## Testing

```bash
make test          # all tests with -race -cover
make lint          # static analysis via golangci-lint
```

- **Domain & Application layer**: unit tests (mock repository interfaces, no DB).
- **Infrastructure layer**: integration tests via testcontainers-go.

## Documentation

Architecture rules & conventions (including for AI coding agents) are documented in [`AGENTS.md`](./AGENTS.md). API specifications, background jobs, and testing strategies are detailed in [`TECHNICAL_DOCUMENTATION.md`](./TECHNICAL_DOCUMENTATION.md). Full product requirements are managed in a separate repository — contact the maintainer if you need access.

## License

All Rights Reserved — see [`LICENSE`](./LICENSE). This repository is public for portfolio/demonstration purposes, not for reuse without permission.
