.PHONY: run-api run-worker migrate wire swag test tidy lint

# Menjalankan aplikasi secara lokal (tanpa Docker)
run-api:
	go run ./cmd/api
run-worker:
	go run ./cmd/worker

# Database Migration
migrate:
	go run ./cmd/migrate

# Generate Dependency Injection (google/wire)
wire:
	wire ./cmd/api
	wire ./cmd/worker

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