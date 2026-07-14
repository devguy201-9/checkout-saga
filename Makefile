## Makefile — checkout-saga
## Workflow: lint + race + vuln gates before pushing.

ifneq (,$(wildcard ./.env))
    include .env
    export
endif

SERVICES := order inventory payment saga notification
BIN_DIR  := bin
COVER    := coverage.txt
DOCKER   := docker compose

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\n"} \
		/^[a-zA-Z_-]+:.*?## / { printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

## --- Docker / Infra ---

.PHONY: up
up: ## Start infra containers (postgres, redis, adminer)
	$(DOCKER) up -d
	@sleep 3
	@$(DOCKER) ps

.PHONY: down
down: ## Stop containers
	$(DOCKER) down

.PHONY: down-v
down-v: ## Stop containers + WIPE volumes (resets DB data)
	$(DOCKER) down -v

.PHONY: logs
logs: ## Tail container logs
	$(DOCKER) logs -f

.PHONY: ps
ps: ## Container status
	$(DOCKER) ps

## --- Verify infra ---

.PHONY: verify-db
verify-db: ## Check 4 databases exist
	@$(DOCKER) exec -T postgres psql -U $(POSTGRES_USER) -d postgres -c "\l" \
		| grep -E "order_db|inventory_db|payment_db|saga_db" \
		|| (echo "DATABASES MISSING. Run: make down-v && make up"; exit 1)
	@echo "  4 databases exist"

.PHONY: verify-redis
verify-redis: ## Check Redis responds
	@$(DOCKER) exec -T redis redis-cli ping

.PHONY: verify
verify: verify-db verify-redis ## Verify all infra healthy
	@echo "  Infra OK"

## --- Build ---

.PHONY: build
build: $(SERVICES:%=build-%) ## Build all 5 services to bin/
	@echo "  All services built"

build-%: ## Build single service
	@mkdir -p $(BIN_DIR)
	@echo "Building $* ..."
	@CGO_ENABLED=0 go build -ldflags='-w -s -X main.version=$(shell git rev-parse --short HEAD 2>/dev/null || echo dev)' \
		-o $(BIN_DIR)/$* ./cmd/$*

## --- Run (local, no docker) ---

run-%: ## Run a service locally (e.g. make run-order)
	go run ./cmd/$*

## --- Test ---

.PHONY: test
test: ## Run unit tests with race detector + coverage
	go test -v -race -covermode=atomic -coverprofile=$(COVER) ./...

.PHONY: test-short
test-short: ## Run only short tests (skip slow ones)
	go test -short -race ./...

.PHONY: cover
cover: test ## Open coverage HTML
	go tool cover -html=$(COVER) -o coverage.html
	@echo "Open coverage.html"

.PHONY: cover-func
cover-func: ## Coverage by function
	@go test -cover -coverprofile=$(COVER) ./... > /dev/null
	@go tool cover -func=$(COVER)

.PHONY: bench
bench: ## Run benchmarks
	go test -bench=. -benchmem -run=^$$ ./...

## --- Quality gates ---

.PHONY: fmt
fmt: ## Format code (gofumpt + goimports)
	@command -v gofumpt >/dev/null || go install mvdan.cc/gofumpt@latest
	@command -v goimports >/dev/null || go install golang.org/x/tools/cmd/goimports@latest
	gofumpt -l -w .
	goimports -w .

.PHONY: lint
lint: ## golangci-lint v2 strict
	@command -v golangci-lint >/dev/null || { \
		echo "Install: https://golangci-lint.run/welcome/install/#local-installation"; exit 1; }
	golangci-lint run ./...

.PHONY: vuln
vuln: ## Check known vulnerabilities (govulncheck)
	@command -v govulncheck >/dev/null || go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

.PHONY: tidy
tidy: ## go mod tidy + verify
	go mod tidy
	go mod verify

.PHONY: pre-commit
pre-commit: fmt lint test vuln ## Run all gates before commit
	@echo "  Pre-commit gates passed"

## --- Cleanup ---

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) $(COVER) coverage.html

## --- Checkpoints ---

.PHONY: checkpoint
checkpoint: ## Verify the foundation is done
	@echo "===== Foundation Checkpoint ====="
	@echo ""
	@echo "1. Containers:"
	@$(DOCKER) ps --status running --format "  - {{.Service}}"
	@echo ""
	@echo "2. Infra:"
	@$(MAKE) -s verify
	@echo ""
	@echo "3. Build:"
	@$(MAKE) -s build && echo "     All 5 services compile"
	@echo ""
	@echo "4. Quality gates:"
	@$(MAKE) -s lint && echo "     lint pass" || echo "   ✗ lint FAIL"
	@$(MAKE) -s test && echo "     tests pass" || echo "   ✗ tests FAIL"
	@echo ""
	@echo "===== Foundation   Done ====="

## --- Database migrations (golang-migrate) ---
include makefile-nextday1.mk