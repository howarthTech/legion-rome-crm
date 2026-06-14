.DEFAULT_GOAL := help
SHELL := /usr/bin/env bash
export PATH := $(HOME)/.local/go/bin:$(PATH)

GO := $(shell command -v go 2>/dev/null || echo $$HOME/.local/go/bin/go)

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Compile bin/server
	@$(GO) build -o bin/server ./cmd/server
	@echo "✓ Built bin/server ($$(du -h bin/server | awk '{print $$1}'))"

.PHONY: docker-build
docker-build: ## Build the container image (legion-rome-crm:dev)
	@docker build -t legion-rome-crm:dev .
	@echo "✓ Built legion-rome-crm:dev ($$(docker images legion-rome-crm:dev --format '{{.Size}}'))"

.PHONY: run
run: build ## Build and run with .env loaded
	@if [ ! -f .env ]; then echo "❌ .env missing. cp .env.example .env first." && exit 1; fi
	@set -a; source ./.env; set +a; ./bin/server

.PHONY: dev
dev: ## Run with hot-reload (requires `air` — install: go install github.com/air-verse/air@latest)
	@command -v air >/dev/null || (echo "Install air: go install github.com/air-verse/air@latest" && exit 1)
	@air

.PHONY: hash-password
hash-password: ## Generate a bcrypt hash for the admin password. Usage: make hash-password PW=mypass
	@if [ -z "$$PW" ]; then echo "Usage: make hash-password PW=your-password" && exit 1; fi
	@$(GO) run ./cmd/hash-password "$$PW"

.PHONY: gen-secret
gen-secret: ## Generate a random SESSION_SECRET
	@head -c 32 /dev/urandom | base64

.PHONY: test
test: ## Run unit tests
	@$(GO) test ./...

.PHONY: lint
lint: ## Run go vet
	@$(GO) vet ./...

.PHONY: clean
clean: ## Remove build output and local DB
	@rm -rf bin/ data/
	@echo "✓ Cleaned bin/ and data/"

.PHONY: tidy
tidy: ## go mod tidy
	@$(GO) mod tidy
