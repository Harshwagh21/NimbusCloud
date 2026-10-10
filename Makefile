.DEFAULT_GOAL := help
.PHONY: help up down logs reset run build test test-unit lint vet fmt tidy migrate-up migrate-down migrate-new migrate-status sqlc tools

MIGRATIONS := db/migrations
DB_URL     := postgres://nimbus:nimbus_dev_password@localhost:5433/nimbus?sslmode=disable

help: ## Show available targets
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n",$$1,$$2}'

## --- infrastructure ---

up: ## Start Postgres + MinIO
	docker compose up -d

down: ## Stop containers, keep data
	docker compose down

logs: ## Tail container logs
	docker compose logs -f

reset: ## Destroy containers AND data, then start fresh
	docker compose down -v
	docker compose up -d

## --- application ---

run: ## Run the API
	go run ./cmd/api

build: ## Build the API binary
	go build -o bin/nimbus-api ./cmd/api

## --- database ---

migrate-up: ## Apply all migrations
	migrate -path $(MIGRATIONS) -database "$(DB_URL)" up

migrate-down: ## Roll back the most recent migration
	migrate -path $(MIGRATIONS) -database "$(DB_URL)" down 1

migrate-status: ## Show the current migration version
	migrate -path $(MIGRATIONS) -database "$(DB_URL)" version

migrate-new: ## Create a migration pair: make migrate-new name=add_users
	migrate create -ext sql -dir $(MIGRATIONS) -seq $(name)

sqlc: ## Regenerate typed queries from db/queries
	sqlc generate

## --- quality ---

test: ## Run all tests
	go test ./... -count=1

test-unit: ## Run tests that need no database
	go test ./... -short -count=1

vet: ## Run go vet
	go vet ./...

lint: vet ## Run go vet + golangci-lint
	golangci-lint run

fmt: ## Format and simplify
	gofmt -s -w .

tidy: ## Tidy module dependencies
	go mod tidy

tools: ## Install the development tools this project needs
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	go install -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
