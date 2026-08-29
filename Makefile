-include .env
export

.DEFAULT_GOAL := help

# One compose file serves every environment; only the env file differs.
# (dev and prod read the same filename — on different machines.)
COMPOSE      := docker compose -f docker/docker-compose.yml
COMPOSE_DEV  := $(COMPOSE) --env-file .env
COMPOSE_PROD := $(COMPOSE) --env-file .env
COMPOSE_STG  := $(COMPOSE) --env-file .env.staging
SHARED_NET   := your-persona-shared

.PHONY: help dev prod stop prune logs run-api run-worker migrate migrate-diff seed build clean wire swag test lint tidy \
	prod-up prod-down prod-restart prod-redeploy prod-logs prod-ps prod-migrate prod-seed \
	staging-up staging-down staging-restart staging-redeploy staging-logs staging-ps staging-migrate staging-seed \
	restart-caddy

dev: ## Start dev environment (Air hot-reload via compose watch + Postgres/Redis/Mailpit)
	@echo "Starting development environment (Air Hot-Reload)..."
	@docker network inspect $(SHARED_NET) >/dev/null 2>&1 || docker network create $(SHARED_NET)
	$(COMPOSE_DEV) watch

prod: ## [LOCAL] Prod-like preview — builds the runtime image but keeps your dev .env; NOT the VPS deploy (see `make prod-*` for that)
	@echo "Starting production-like preview environment (local only)..."
	BUILD_TARGET=runtime API_COMMAND=./api WORKER_COMMAND=./worker $(COMPOSE_DEV) up -d --build

stop: ## Stop all Docker services
	@echo "Stopping all services..."
	$(COMPOSE_DEV) down

prune: ## Stop and remove all containers + volumes (WARNING: DB data lost)
	@echo "Stopping and removing all containers and volumes..."
	$(COMPOSE_DEV) down -v

logs: ## Tail Docker service logs (usage: make logs or make logs s=api)
	$(COMPOSE_DEV) logs -f $(if $(s),$(s),)

run-api: ## Run API server locally
	go run ./cmd/api

run-worker: ## Run Asynq worker locally
	go run ./cmd/worker

migrate: ## Apply pending migrations to the DB (Atlas; needs atlas CLI on host)
	go run ./cmd/migrate

migrate-diff: ## Generate a migration from struct changes (usage: make migrate-diff [name=add_x]; name optional but recommended; needs Docker + atlas CLI)
	atlas migrate diff $(name) --env gorm

seed: ## Seed database with initial data (questions, templates, etc.)
	go run ./cmd/seed

build: ## Compile all binaries to ./bin/
	@echo "Building all binaries..."
	@mkdir -p bin
	go build -o bin/api ./cmd/api
	go build -o bin/worker ./cmd/worker
	go build -o bin/migrate ./cmd/migrate
	go build -o bin/seed ./cmd/seed

clean: ## Remove build artifacts (bin/, tmp/, *.exe)
	@echo "Cleaning build artifacts..."
	rm -rf bin/ tmp/
	rm -f api.exe worker.exe migrate.exe seed.exe
	rm -f api worker migrate seed

wire: ## Regenerate dependency injection (google/wire)
	@echo "Generating wire_gen.go..."
	go run github.com/google/wire/cmd/wire ./cmd/api
	go run github.com/google/wire/cmd/wire ./cmd/worker

swag: ## Regenerate Swagger API documentation
	@echo "Generating Swagger documentation..."
	cd cmd/api && go run github.com/swaggo/swag/cmd/swag init -g main.go -o ../../docs --parseInternal -d .,../../internal,../../pkg

test: ## Run all tests with race detector + coverage
	@echo "Running tests..."
	go test ./... -race -cover

lint: ## Run golangci-lint
	golangci-lint run

tidy: ## Tidy go.mod and go.sum
	go mod tidy

## --- VPS ops (run these ON THE VPS, inside the matching checkout dir —
## /opt/your-persona/controller-api for prod-*, .../controller-api-staging for
## staging-* — not on a local dev machine) ---

prod-up: ## [VPS] Pull latest image + (re)create prod containers
	$(COMPOSE_PROD) pull
	$(COMPOSE_PROD) up -d --no-build --remove-orphans

prod-down: ## [VPS] Stop prod containers (keeps volumes/data)
	$(COMPOSE_PROD) down

prod-restart: ## [VPS] Restart prod containers without pulling a new image (usage: make prod-restart [s=caddy])
	$(COMPOSE_PROD) restart $(if $(s),$(s),)

prod-redeploy: ## [VPS] Full redeploy: git reset to origin/main + pull image + recreate (DESTRUCTIVE git reset --hard — VPS only, never run this on a dev machine)
	git fetch origin main
	git reset --hard origin/main
	$(MAKE) prod-up
	docker image prune -f

prod-logs: ## [VPS] Tail prod logs (usage: make prod-logs [s=api])
	$(COMPOSE_PROD) logs -f --tail=200 $(if $(s),$(s),)

prod-ps: ## [VPS] Show prod container status
	$(COMPOSE_PROD) ps

prod-migrate: ## [VPS] Apply pending Atlas migrations against prod DB
	$(COMPOSE_PROD) run --rm api ./migrate

prod-seed: ## [VPS] Seed prod DB (idempotent — question bank + insight templates)
	$(COMPOSE_PROD) run --rm api ./seed

staging-up: ## [VPS] Pull latest image + (re)create staging containers
	$(COMPOSE_STG) pull
	$(COMPOSE_STG) up -d --no-build --remove-orphans

staging-down: ## [VPS] Stop staging containers (keeps volumes/data)
	$(COMPOSE_STG) down

staging-restart: ## [VPS] Restart staging containers without pulling a new image (usage: make staging-restart [s=api-staging])
	$(COMPOSE_STG) restart $(if $(s),$(s),)

staging-redeploy: ## [VPS] Full redeploy: git reset to origin/develop + pull image + recreate (DESTRUCTIVE git reset --hard — VPS only, never run this on a dev machine)
	git fetch origin develop
	git reset --hard origin/develop
	$(MAKE) staging-up
	docker image prune -f

staging-logs: ## [VPS] Tail staging logs (usage: make staging-logs [s=api-staging])
	$(COMPOSE_STG) logs -f --tail=200 $(if $(s),$(s),)

staging-ps: ## [VPS] Show staging container status
	$(COMPOSE_STG) ps

staging-migrate: ## [VPS] Apply pending Atlas migrations against staging DB
	$(COMPOSE_STG) run --rm api ./migrate

staging-seed: ## [VPS] Seed staging DB (idempotent — question bank + insight templates)
	$(COMPOSE_STG) run --rm api ./seed

restart-caddy: ## [VPS] Restart the shared Caddy (fixes stuck ACME/TLS retry backoff — a recurring gotcha, see DEPLOYMENT-GUIDE.md) — run from the prod checkout dir, Caddy is the `edge` profile enabled by that .env
	$(COMPOSE_PROD) restart caddy

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'