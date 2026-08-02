# Controller API — Your Persona's

The backend engine of **Your Persona's** — a Go service that powers an AI-driven psychological assessment platform (MBTI-style + GRIT), orchestrating essay analysis via Gemini API, async PDF generation, and a privacy-first data lifecycle.

---

## Technical Architecture

This service is structured using **Clean Architecture + Domain-Driven Design (DDD)**:

* **Domain** (`internal/domain`): Framework-agnostic core entities (`User`, `TestResult`, `GuestSession`, `DeletionRequest`) and repository interfaces. Imports nothing from GORM/HTTP/SDKs.
* **Application** (`internal/application`): Business workflows (assessment submission + scoring, auth/OTP flows, quota enforcement, referral events, anonymization orchestration).
* **Interfaces** (`internal/interfaces`): Two parallel delivery layers calling the same use cases — HTTP REST handlers (Echo) and Asynq worker handlers.
* **Infrastructure** (`internal/infrastructure`): Third-party adapters — PostgreSQL (GORM), Redis, Cloudflare R2/MinIO, Gemini API, SMTP (`mail.digitalsekuriti.id` prod / Mailpit dev).

```mermaid
flowchart LR
    Client[Browser / FE] -->|HTTPS| Caddy[Caddy<br/>TLS + reverse proxy]
    Caddy --> API["cmd/api<br/>(Echo HTTP server)"]

    subgraph Core["Clean Architecture core"]
        direction TB
        IF[internal/interfaces<br/>HTTP + Worker handlers] --> APP[internal/application<br/>use cases]
        APP --> DOM[internal/domain<br/>entities + repo interfaces]
    end

    API --> IF
    Worker["cmd/worker<br/>(Asynq: pdf / io servers)"] --> IF

    APP --> INFRA[internal/infrastructure]
    INFRA --> PG[(PostgreSQL)]
    INFRA --> RD[(Redis<br/>cache · rate-limit · queue)]
    INFRA --> R2[(R2 / MinIO<br/>PDF storage)]
    INFRA --> GEM[Gemini API]
    INFRA --> SMTP[Brevo / Mailpit]

    API -.enqueue jobs.-> RD
    RD -.pdf · email · anonymize · purge.-> Worker
```

---

## Directory Structure

```
controller-api/
├── cmd/
│   ├── api/                 # HTTP server entrypoint + Wire DI config
│   ├── worker/              # Asynq background worker entrypoint
│   ├── migrate/             # Applies Atlas versioned migrations (manual-only, never at boot)
│   ├── atlasloader/         # Dev-only: emits GORM schema as SQL for `atlas migrate diff`
│   └── seed/                # Question bank & insight template seeder (idempotent)
├── migrations/              # Atlas versioned migration files (see migrations/README.md)
├── docs/                    # Swagger files — swagger.json = official FE contract
├── internal/
│   ├── domain/              # Entities and repository contracts
│   ├── application/         # Use case orchestrators
│   ├── infrastructure/      # Postgres, Redis, R2, Gemini, SMTP, i18n adapters
│   └── interfaces/          # HTTP handlers (Echo) + Worker handlers (Asynq)
├── pkg/                     # Shared utilities (httpresponse, locale, taskqueue, aivalidator)
├── docker/                  # Dockerfile, Dockerfile.dev, compose files, Caddyfile
├── scripts/                 # Ops scripts (DB backup to S3/R2)
├── .github/workflows/       # CI/CD pipeline (7 jobs)
└── Makefile                 # Build and development task runner
```

---

## Setup & Running Locally

### Prerequisites
* Go 1.26+
* Docker & Docker Compose
* `make` tool
* [Atlas CLI](https://atlasgo.io) — for generating/applying schema migrations (`curl -sSf https://atlasgo.sh | sh`); the Docker image bundles it, so this is only needed for local migration work

### Step 1 — Environment Settings
Copy `.env.example` to `.env` and fill in the necessary fields. In production mode (`APP_ENV=production`), the server enforces strict boot validation — it refuses to start if secrets are still dev defaults or required keys are empty.

Key variables:
* `JWT_SECRET` — token signing key
* `GEMINI_API_KEY` / `GEMINI_MODEL` — model **must** be a pinned version (e.g. `-001`), never an auto-updating alias
* `ALLOWED_ORIGINS` — comma-separated CORS whitelist; `*` is rejected at boot
* `TURNSTILE_SECRET_KEY` — empty in dev = bot-check auto-passes

### Step 2 — Run Service Stack
```bash
make dev    # Postgres, Redis, MinIO, Mailpit + Air hot-reload
```

### Step 3 — Migrate & Seed
```bash
make migrate    # applies Atlas versioned migrations (never auto-runs — by design)
make seed       # question bank + insight templates
```
Schema changes are versioned with Atlas from the GORM structs — see [`migrations/README.md`](./migrations/README.md) for the `atlas migrate diff` workflow. (Switched from GORM AutoMigrate on 2026-07-25.)

Dev tools available at:
* **Swagger UI**: http://localhost:8080/swagger/index.html
* **Mailpit** (email catcher): http://localhost:8025
* **MinIO Console** (S3 storage): http://localhost:9001 (`minioadmin`/`minioadmin`)

---

## Core Operations & Features

### 1. Synchronous AI Assessment Pipeline
* `POST /v1/assessment/submit` calls Gemini **synchronously** (3–8s, by design — the FE "Waiting Room" UX) with role-separated prompts and structural essay framing (prompt-injection mitigations).
* Layered cost protection ("Denial of Wallet"): 32KB payload cap, 4,000-char essay cap, garbage-input filter, per-IP rate limit, Redis distributed quota lock (`quota_lock:<id>`), in-process Gemini semaphore, and a global daily token budget — exhaustion degrades gracefully to a static fallback result, never an error.
* Every Gemini call is audited to `PROMPT_AUDIT_LOG` (30-day TTL, auto-purged).

### 2. Background Jobs (Asynq)
* Two Asynq servers in one worker process: `pdf` queue (CPU-bound, capped concurrency) isolated from io queues (`critical`/`default`/`low`) — a PDF burst can never starve OTP emails.
* Job types: `generate:pdf`, `send:email`, `anonymize:user`, plus scheduled scans (`deletion:scan-expired` hourly, `purge:guest-ttl` + `purge:audit-ttl` daily). All idempotent.

### 3. Auth & Account Security
* JWT with rotation + denylist, `token_version` mass-revocation, account lockout separate from per-IP rate limits, HIBP breach-check on passwords, Cloudflare Turnstile on register/login/forgot-password (fail-open if Cloudflare itself is down), CSRF double-submit on cookie-sensitive endpoints.

### 4. Privacy-First Data Lifecycle
* Account deletion = 14-day grace period → **anonymization** (not hard-delete): PII scrambled, aggregate stats retained, R2 PDFs explicitly deleted.
* Guest results auto-expire after 14 days (R2 object deleted **before** DB row — no orphan files).

---

## CI/CD & Branching

```mermaid
flowchart LR
    subgraph CI["CI — every push & PR (.github/workflows/ci.yml)"]
        direction LR
        SEC[secrets<br/>gitleaks] ~~~ L[lint]
        L --> T[test] & I[integration] & S[security<br/>govulncheck + gosec]
        T & I & S --> B[build<br/>Docker → GHCR]
        SEC --> B
    end
    B -->|"push to main only"| D[deploy<br/>SSH pull & recreate + smoke test]
```

```mermaid
flowchart LR
    W["update_july_week_N<br/>(weekly feature branch)"] -->|PR + 6 checks| DEV[develop<br/>staging]
    DEV -->|release PR| MAIN[main<br/>production]
    MAIN -->|"tag vX.Y.Z"| REL[GitHub Release]
    MAIN -->|CI deploy job| VPS[VPS]
```

`main` and `develop` are **branch-protected**: no direct pushes, no force-push, no deletion — every change lands via a PR passing all 6 required checks on an up-to-date branch. Releases: PR `develop` → `main`, then SemVer tag + GitHub Release.

**Deploy (live since 2026-07-25):** push to `main` → `deploy` job **pauses for owner approval** (GitHub `production` environment gate) → SSH pull & recreate + smoke test. Push to `develop` → `deploy-staging` job auto-deploys to the staging stack (no approval). Both target one Oracle Cloud Ampere A1 VPS (arm64); the image is built **natively for `linux/arm64` only** on a `ubuntu-24.04-arm` runner (`linux/amd64` was dropped 2026-08-02 — it was never pulled by anything, and cross-building arm64 under QEMU cost several minutes per run). **Migrations never auto-run on deploy** — see [`docs/deploy_runbook.md`](./docs/deploy_runbook.md).

**Versioning:** automated via [`release-please`](https://github.com/googleapis/release-please) (`.github/workflows/release-please.yml`). Pushing to `main` opens a release PR that bumps the version and, once merged, creates the git tag + GitHub Release. The bump is derived from Conventional Commits — and specifically from the **merge-commit title (= PR title)**, so a promotion PR must be titled `feat:`/`fix:`, never `release:`/`chore:` (those are not "user facing" and the release is silently skipped). `CHANGELOG.md` is hand-maintained and deliberately untouched by the tool (`skip-changelog: true`).

---

## Testing

```bash
make test                         # unit only — fast, no Docker
go test -tags=integration ./...   # + testcontainers (needs Docker running)
```

* **Domain/application**: unit tests with mockery-generated mocks (checked into git; regenerate via `mockery`). A test needing a real DB in these layers is in the wrong layer.
* **Infrastructure**: Postgres/Redis code uses integration tests (testcontainers-go, behind `//go:build integration`); HTTP clients test against `httptest.Server`; vendor-SDK wrappers are tested on pre-flight logic only.
* **Interfaces**: real use case + mocked lower deps — validates status codes, envelope shape, cookie/header handling. Handler helpers signal an already-written response via the `errResponseWritten` sentinel (`helpers.go`).
* CI runs the full suite **including integration** on every push/PR.

---

## API Contract

`docs/swagger.json` (+ `swagger.yaml`) is the machine-readable contract, generated from handler annotations (`make swag`) and committed to git. The FE builds against this file — not by reading Go source. **Breaking changes to `/v1` go through `/v2`, never silent edits**; additive changes (new optional field/endpoint) are fine.

---

## Makefile Quick Reference

| Command | Description |
|---|---|
| `make dev` | Starts dev environment with Air live-reload (Postgres/Redis/MinIO/Mailpit). |
| `make prod` | **Local** prod-like preview via `docker-compose.yml` (no Caddy/TLS) — NOT the VPS deploy, see below. |
| `make stop` / `make prune` | Stops containers / stops + wipes volumes (**DB data lost**). |
| `make migrate` / `make seed` | Applies schema migration / seeds initial data. |
| `make wire` | Regenerates dependency injection (`wire_gen.go`). |
| `make swag` | Regenerates Swagger API documentation. |
| `make test` / `make lint` | Unit tests (race + coverage) / golangci-lint. |
| `make run-api` / `make run-worker` | Runs binaries locally without Docker. |

**VPS ops** (run these ON THE VPS, not on a dev machine — see [`docs/deploy_runbook.md`](./docs/deploy_runbook.md)):

| Command | Description |
|---|---|
| `make prod-up` / `make staging-up` | Pull latest image + (re)create containers for that environment. |
| `make prod-restart [s=<service>]` / `make staging-restart [s=<service>]` | Restart without pulling a new image (optionally scoped to one service). |
| `make prod-redeploy` / `make staging-redeploy` | Full redeploy: `git reset --hard` to the tracked branch + pull image + recreate. **Destructive** (discards local changes) — this is what the CI `deploy`/`deploy-staging` jobs run over SSH. |
| `make prod-logs [s=<service>]` / `make staging-logs [s=<service>]` | Tail logs, optionally scoped to one service. |
| `make prod-ps` / `make staging-ps` | Container status. |
| `make prod-migrate` / `make staging-migrate` | Apply pending migrations against that environment's DB. |
| `make prod-seed` / `make staging-seed` | Seed that environment's DB (idempotent). |
| `make restart-caddy` | Restart the shared Caddy — fixes a stuck ACME/TLS retry backoff (recurring gotcha, see `DEPLOYMENT-GUIDE.md`). |

---

## Documentation

* [`AGENTS.md`](./AGENTS.md) — architecture, security & git-workflow rules (AI agents included)
* [`TECHNICAL_DOCUMENTATION.md`](./TECHNICAL_DOCUMENTATION.md) — API spec, background jobs, testing strategy
* [`docs/deploy_runbook.md`](./docs/deploy_runbook.md) — production deploy, redeploy, rollback
* Product requirements live in a separate private repo — contact the maintainer for access.

## License

All Rights Reserved — see [`LICENSE`](./LICENSE). Public for portfolio/demonstration purposes, not for reuse without permission.
