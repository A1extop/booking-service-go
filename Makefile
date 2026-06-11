COMPOSE_FILE := dev/deployments/docker-compose.yml
MIGRATE_DSN := postgres://booking:booking@localhost:5433/booking?sslmode=disable
GOLANGCI_LINT_BIN := $(shell go env GOPATH)/bin/golangci-lint
GOVERSION := $(shell go env GOVERSION)

.PHONY: up down migrate tests integration-tests coverage lint

up:
	docker compose -f $(COMPOSE_FILE) up -d --build

tests:
	go test ./... | grep -v "no test files"

integration-tests:
	go test -tags=integration -v ./storage/postgres/...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

migrate:
	POSTGRES_DSN=$(MIGRATE_DSN) go run ./cmd/migrator

down:
	docker compose -f $(COMPOSE_FILE) down

lint:
	@[ -f $(GOLANGCI_LINT_BIN) ] || GOTOOLCHAIN=$(GOVERSION) go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GOLANGCI_LINT_BIN) run ./...
