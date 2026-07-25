-include .env
export

.DEFAULT_GOAL := help

.PHONY: help dev prod stop prune logs run-api run-worker migrate migrate-diff seed build clean wire swag test lint tidy \
	prod-up prod-down prod-restart prod-redeploy prod-logs prod-ps prod-migrate prod-seed \
	staging-up staging-down staging-restart staging-redeploy staging-logs staging-ps staging-migrate staging-seed \
	restart-caddy

dev: ## Start dev environment (Air hot-reload + Postgres/Redis/MinIO/Mailpit)
	@echo "Starting development environment (Air Hot-Reload)..."
	docker compose -f docker/docker-compose.yml -f docker/docker-compose.dev.yml up --build

prod: ## [LOCAL] Prod-like preview via docker-compose.yml — NOT the VPS deploy, no Caddy/TLS/shared network (see `make prod-*` targets below for that)
	@echo "Starting production-like preview environment (local only)..."
	docker compose -f docker/docker-compose.yml up -d --build

stop: ## Stop all Docker services
	@echo "Stopping all services..."
	docker compose -f docker/docker-compose.yml -f docker/docker-compose.dev.yml down

prune: ## Stop and remove all containers + volumes (WARNING: DB data lost)
	@echo "Stopping and removing all containers and volumes..."
	docker compose -f docker/docker-compose.yml -f docker/docker-compose.dev.yml down -v

logs: ## Tail Docker service logs (usage: make logs or make logs s=api)
	docker compose -f docker/docker-compose.yml -f docker/docker-compose.dev.yml logs -f $(if $(s),$(s),)

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
	docker compose -f docker/docker-compose.prod.yml --env-file .env pull
	docker compose -f docker/docker-compose.prod.yml --env-file .env up -d --no-build --remove-orphans

prod-down: ## [VPS] Stop prod containers (keeps volumes/data)
	docker compose -f docker/docker-compose.prod.yml --env-file .env down

prod-restart: ## [VPS] Restart prod containers without pulling a new image (usage: make prod-restart [s=caddy])
	docker compose -f docker/docker-compose.prod.yml --env-file .env restart $(if $(s),$(s),)

prod-redeploy: ## [VPS] Full redeploy: git reset to origin/main + pull image + recreate (DESTRUCTIVE git reset --hard — VPS only, never run this on a dev machine)
	git fetch origin main
	git reset --hard origin/main
	$(MAKE) prod-up
	docker image prune -f

prod-logs: ## [VPS] Tail prod logs (usage: make prod-logs [s=api])
	docker compose -f docker/docker-compose.prod.yml --env-file .env logs -f --tail=200 $(if $(s),$(s),)

prod-ps: ## [VPS] Show prod container status
	docker compose -f docker/docker-compose.prod.yml --env-file .env ps

prod-migrate: ## [VPS] Apply pending Atlas migrations against prod DB
	docker compose -f docker/docker-compose.prod.yml --env-file .env run --rm api ./migrate

prod-seed: ## [VPS] Seed prod DB (idempotent — question bank + insight templates)
	docker compose -f docker/docker-compose.prod.yml --env-file .env run --rm api ./seed

staging-up: ## [VPS] Pull latest image + (re)create staging containers
	docker compose -f docker/docker-compose.staging.yml --env-file .env.staging pull
	docker compose -f docker/docker-compose.staging.yml --env-file .env.staging up -d --no-build --remove-orphans

staging-down: ## [VPS] Stop staging containers (keeps volumes/data)
	docker compose -f docker/docker-compose.staging.yml --env-file .env.staging down

staging-restart: ## [VPS] Restart staging containers without pulling a new image (usage: make staging-restart [s=api-staging])
	docker compose -f docker/docker-compose.staging.yml --env-file .env.staging restart $(if $(s),$(s),)

staging-redeploy: ## [VPS] Full redeploy: git reset to origin/develop + pull image + recreate (DESTRUCTIVE git reset --hard — VPS only, never run this on a dev machine)
	git fetch origin develop
	git reset --hard origin/develop
	$(MAKE) staging-up
	docker image prune -f

staging-logs: ## [VPS] Tail staging logs (usage: make staging-logs [s=api-staging])
	docker compose -f docker/docker-compose.staging.yml --env-file .env.staging logs -f --tail=200 $(if $(s),$(s),)

staging-ps: ## [VPS] Show staging container status
	docker compose -f docker/docker-compose.staging.yml --env-file .env.staging ps

staging-migrate: ## [VPS] Apply pending Atlas migrations against staging DB
	docker compose -f docker/docker-compose.staging.yml --env-file .env.staging run --rm api-staging ./migrate

staging-seed: ## [VPS] Seed staging DB (idempotent — question bank + insight templates)
	docker compose -f docker/docker-compose.staging.yml --env-file .env.staging run --rm api-staging ./seed

restart-caddy: ## [VPS] Restart the shared Caddy (fixes stuck ACME/TLS retry backoff — a recurring gotcha, see DEPLOYMENT-GUIDE.md) — run from the prod checkout dir, Caddy lives in docker-compose.prod.yml
	docker compose -f docker/docker-compose.prod.yml --env-file .env restart caddy

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'