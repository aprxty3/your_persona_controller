-include .env
export

.PHONY: dev prod stop prune run-api run-worker migrate wire swag test tidy lint

# Starts the development environment with live reload using Docker.
dev:
	@echo "Starting development environment (Air Hot-Reload)..."
	docker compose -f docker/docker-compose.yml -f docker/docker-compose.dev.yml up --build

# Starts the production environment in detached mode.
prod:
	@echo "Starting production environment..."
	docker compose -f docker/docker-compose.yml up -d --build

# Stops all services.
stop:
	@echo "Stopping all services..."
	docker compose -f docker/docker-compose.yml -f docker/docker-compose.dev.yml down

# Stop and remove all containers, networks, and volumes (WARNING: DB data will be lost).
prune:
	@echo "Stopping and removing all containers and volumes..."
	docker compose -f docker/docker-compose.yml -f docker/docker-compose.dev.yml down -v

# Run application locally (without Docker)
run-api:
	go run ./cmd/api
run-worker:
	go run ./cmd/worker

# Database Migration
migrate:
	go run ./cmd/migrate

# Generate Dependency Injection (google/wire)
# Using 'go run' to ensure the correct version is used without requiring a global CLI installation
wire:
	go run github.com/google/wire/cmd/wire ./cmd/api
	go run github.com/google/wire/cmd/wire ./cmd/worker

# Generate Swagger API Documentation
# Using 'go run' to ensure the correct version is used without requiring a global CLI installation
swag:
	@echo "Generating Swagger documentation via go run..."
	cd cmd/api && go run github.com/swaggo/swag/cmd/swag@latest init -g main.go -o ../../docs --parseInternal -d .,../../internal,../../pkg

# Testing & Linter
test:
	go test ./... -race -cover
lint:
	golangci-lint run
tidy:
	go mod tidy