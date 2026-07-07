# controller-api

Backend API (Go) for **Your Persona's** — an AI-powered psychological assessment platform.

## Stack

Go · PostgreSQL (GORM) · Redis · Asynq · Gemini API · Cloudflare R2 · Clean Architecture + DDD

## Running Locally

```bash
cp .env.example .env
docker compose -f docker/docker-compose.yml -f docker/docker-compose.dev.yml up
go run ./cmd/migrate
go run ./cmd/api      # or: air, for hot-reload
go run ./cmd/worker
```

- Mailpit (dev email catcher): http://localhost:8025
- MinIO console (S3-compatible dev storage): http://localhost:9001

## Testing

```bash
go test ./...
```

## Dependency Injection (Wire)

This project uses **Google Wire** for *compile-time dependency injection*. 
- **`wire.go`**: Contains dependency injection configuration (`//go:build wireinject`).
- **`wire_gen.go`**: Auto-generated code. **Do not modify this file manually.**

If you add a new repository, use case, or handler constructor:
1. Register the new constructor in `wire.Build` within `cmd/api/wire.go` or `cmd/worker/wire.go`.
2. Run the command below to regenerate dependencies:
   ```bash
   make wire
   ```
   *(Internally, this target runs `go run github.com/google/wire/cmd/wire ./cmd/api`).*

## Entrypoints (cmd/)

This project has 3 main entrypoints in the `cmd/` directory:

1. **`cmd/api/` (API Server)**: Runs the primary HTTP API server (Echo framework).
2. **`cmd/migrate/` (Database Migrator)**: Runs database schema migration separately (GORM AutoMigrate).
   - **Development Guidelines**: Only needs modification if you are **creating a new table/model** (a new GORM Model struct in `internal/infrastructure/persistence/postgres/models.go`) by adding it to the `db.AutoMigrate(...)` list. Adding new columns or changing existing columns on already registered tables does not require modifications here.
3. **`cmd/worker/` (Background Job Worker)**: Runs the worker process to consume asynchronous task queues (Asynq + Redis) like sending emails, PDF generation, data anonymization, etc.
   - **Development Guidelines**: Needs modification if you are **adding a new background task** by registering it in the worker router (`asynq.ServeMux` in `cmd/worker/main.go`) and regenerating the worker dependency injection.

## Documentation

Architecture rules & conventions for contributions (including for AI coding agents) are documented in [`AGENTS.md`](./AGENTS.md). API specifications, background jobs, and testing strategies are detailed in [`TECHNICAL_DOCUMENTATION.md`](./TECHNICAL_DOCUMENTATION.md). Full product requirements are managed in a separate repository — contact the maintainer if you need access.

## License

All Rights Reserved — see [`LICENSE`](./LICENSE). This repository is public for portfolio/demonstration purposes, not for reuse without permission.
