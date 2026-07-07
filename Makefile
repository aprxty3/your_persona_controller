.PHONY: run-api run-worker migrate wire swag test tidy lint

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
swag:
	swag init -g cmd/api/main.go -o docs

# Testing & Linter
test:
	go test ./... -race -cover
lint:
	golangci-lint run
tidy:
	go mod tidy