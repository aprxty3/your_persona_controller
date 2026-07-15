-include .env
export

.DEFAULT_GOAL := help

.PHONY: help dev prod stop prune logs run-api run-worker migrate seed build clean wire swag test lint tidy

dev: ## Start dev environment (Air hot-reload + Postgres/Redis/MinIO/Mailpit)
	@echo "Starting development environment (Air Hot-Reload)..."
	docker compose -f docker/docker-compose.yml -f docker/docker-compose.dev.yml up --build

prod: ## Start production environment (detached)
	@echo "Starting production environment..."
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

migrate: ## Run database migration (GORM AutoMigrate)
	go run ./cmd/migrate

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

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'