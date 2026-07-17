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
make test                              # unit tests only — fast, no Docker required
go test -tags=integration ./...        # + infrastructure integration tests (needs Docker running)
make lint                              # static analysis via golangci-lint
```

- **Domain & Application layer**: unit tests, using [mockery](https://github.com/vektra/mockery)-generated mocks for every repository/service interface. Never open a real DB/Redis connection; if a test needs one, it's in the wrong layer.
- **Infrastructure layer**: split by what the code actually talks to.
  - `persistence/postgres/*` and `cache/redis/*` wrap GORM/redis calls whose correctness can only be verified against a real engine — mocking the DB/Redis client would only test the mock. These use **integration tests** via [testcontainers-go](https://golang.testcontainers.org/), gated behind the `integration` build tag (`//go:build integration`) so `make test`/`go test ./...` stays fast and Docker-free. Requires a local Docker daemon; each package spins up ONE shared container per `go test` run (see `TestMain` in `internal/infrastructure/persistence/postgres/assessment/integration_test.go` and `internal/infrastructure/cache/redis/integration_test.go`) rather than one per test.
  - Everything else — HTTP-calling clients (`security/hibp.go`, `security/turnstile.go`), pure logic (`jwt`), and adapters over an already-mocked interface (`queue/asynq/pdf_queue_service.go` over `pkg/taskqueue.Dispatcher`) — has plain **unit tests** (`make test`, no build tag, no Docker). HTTP clients are tested against an `httptest.Server` rather than mocked (there's no interface to mock — see the `rangeURL`/`verifyURL` field seam in `hibp.go`/`turnstile.go`); `mailer.go` swaps `smtp.SendMail` for a `sendFunc` field seam so `SendEmail`/`SendOTP`/`SendDeletionConfirmed` can be tested without a real SMTP server, using the real `i18n.Catalog` (loaded from the embedded JSON) so locale copy stays in sync. `queue/asynq/pdf_queue_service.go` reuses the existing `pkg/taskqueue/mocks.MockDispatcher`. Vendor-SDK wrappers with no seam for injecting a fake backend (`gemini`, `storage/s3`) are tested only on their pure/pre-flight logic (prompt building, unconfigured-client guard, endpoint parsing, URL-to-key extraction) — the SDK calls themselves aren't unit-testable without introducing an interface purely for tests, which isn't worth the abstraction here. `queue/asynq.AsynqDispatcher`'s `enqueue` is skipped for the same reason as `persistence/postgres`/`cache/redis` above — nothing to unit-test that isn't a mock of the mock.
- **Interfaces layer** (`internal/interfaces/http`, `internal/interfaces/worker`) and **`pkg/*`**: unit tests throughout, `make test`, no build tag.
  - `pkg/aivalidator`, `pkg/httpresponse`, `pkg/locale`, `pkg/otp` are pure logic — tested directly, no mocks.
  - `pkg/logger` is tested by redirecting `os.Stdout` through a pipe for the duration of the test and asserting on what `NewLogger` actually wrote (format, level filtering, `With()` field binding) — the only way to observe it, since it hardcodes `os.Stdout` as its sink.
  - `internal/interfaces/http/middleware` (`AuthMiddleware`, `LocaleMiddleware`) depends on `account.UserRepository` (an interface — mocked via the existing `internal/domain/account/mocks`) and a real `*jwtservice.JWTService` (pure, no mock needed).
  - `internal/interfaces/http/handler/*` and `internal/interfaces/worker/*` are the one layer in this codebase where the consumer (HTTP handler / asynq job handler) depends on a **concrete use-case struct**, not an interface — so mockery can't mock the use case itself the way it mocks a repository. Instead, each test wires the *real* use case (via its `New...` constructor) with its lower-level repository/service dependencies mocked — the same mocks `internal/application/*`'s own tests use. This validates a different concern than the application-layer tests: HTTP status code / error code mapping, JSON envelope shape, cookie/header handling, asynq retry-vs-drop semantics — not business logic (already covered where the use case is tested directly). Untouched use-case dependencies are left `nil`; per the same convention as `internal/application`, no test constructs a real `*gorm.DB`, so only code paths that don't reach a use case's own `db.Transaction(...)` call are exercised this way.
- CI runs `make test` (unit only) on every push. The `integration` tag is **not** wired into CI yet (would need Docker-in-Docker on the runner) — run it locally before merging changes to `internal/infrastructure/persistence/postgres/` or `internal/infrastructure/cache/redis/`.

### Bugs found while writing handler tests

Writing `internal/interfaces/http/handler` tests surfaced (and fixed) a real bug class, not just test gaps: `httpresponse.Error`/`Success` return `c.JSON(...)`'s own result, which is `nil` on any *successfully-written* response — success or error alike, since it's just an I/O-failure signal. Three helpers wrongly handed that value back to their caller as if it meant "did the request get rejected?": `bindJSON`, `AuthHandler.verifyTurnstile`, and `AuthHandler.otpVerifyError`. A caller checking `if err != nil { return err }` after one of these never actually triggered on rejection, so the handler fell through to real business logic (and, in one path, a second `c.JSON` write on top of the first — Echo doesn't guard body writes past the first commit, so the two responses would concatenate into invalid JSON on the wire). All three now return a shared `errResponseWritten` sentinel instead, so the `!= nil` check callers already had works as intended. See `internal/interfaces/http/handler/helpers.go`.

### Regenerating mocks

Mocks are generated by [mockery](https://github.com/vektra/mockery) (`.mockery.yaml` at the repo root) and **checked into git** (same convention as `wire_gen.go`/`docs/` — reviewable, no local codegen step required to build). After adding or changing an interface under `internal/domain/*`, `internal/application/*`, `internal/infrastructure/mailer`, or `pkg/taskqueue`, regenerate:

```bash
mockery
```

Every package's mocks live in the same place: a plain `pkgname/mocks` subpackage (`with-expecter: true`, `mocks.NewMockX(t)` + `.EXPECT()...Return()`). `internal/application/assessment` and `internal/application/pdf` used to be exceptions — their `IdempotencyService`/`PDFRenderer` interfaces referenced a type defined in their own package (`SubmitResponse`, `PDFData`), which would force a subpackage mock to import back into the package under test and hit Go's "import cycle not allowed in test". Fixed at the source instead of working around it in the mock config: those two response/data types now live in their own leaf `dto` subpackage (`internal/application/assessment/dto`, `internal/application/pdf/dto`) that nothing imports back — so every package, including these two, uses the identical `pkgname/mocks` layout.

## API Contract (Swagger/OpenAPI)

`docs/swagger.json` (and the equivalent `swagger.yaml`) is the machine-readable API contract — generated from the `@Summary`/`@Param`/`@Success`/`@Failure` annotations on every handler in `internal/interfaces/http/handler/`, committed to git (same convention as `wire_gen.go`: reviewable, no local codegen step required to consume it). The frontend (`your_persona_ui`) is expected to build against this file directly — e.g. via [`openapi-typescript`](https://github.com/openapi-ts/openapi-typescript) for generated types/client — rather than hand-transcribing endpoint shapes from reading Go source. Browse it interactively at `/swagger/index.html` on a running instance.

**Regenerate after touching any handler annotation:**
```bash
make swag
```

**This file is a contract, not a draft**: since BE MVP feature-complete gate, a breaking change to any endpoint already in `docs/swagger.json` — removing/renaming a field, changing a status code's meaning, tightening a previously-optional field to required — is a breaking-contract change and must go through `/v2`, not a silent edit to `/v1`. Additive changes (a new optional field, a new endpoint) are fine on `/v1`.

## Documentation

Architecture rules & conventions (including for AI coding agents) are documented in [`AGENTS.md`](./AGENTS.md). API specifications, background jobs, and testing strategies are detailed in [`TECHNICAL_DOCUMENTATION.md`](./TECHNICAL_DOCUMENTATION.md). Production deployment steps are in [`docs/deploy_runbook.md`](./docs/deploy_runbook.md). Full product requirements are managed in a separate repository — contact the maintainer if you need access.

## License

All Rights Reserved — see [`LICENSE`](./LICENSE). This repository is public for portfolio/demonstration purposes, not for reuse without permission.
