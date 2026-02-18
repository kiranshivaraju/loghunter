.PHONY: run test test-unit test-integration lint migrate-up migrate-down build docker-build coverage clean

# ─── Config ──────────────────────────────────────────────────────────────────

MODULE       := github.com/kiranshivaraju/loghunter
BIN_DIR      := bin
SERVER_BIN   := $(BIN_DIR)/loghunter-server
CLI_BIN      := $(BIN_DIR)/loghunter
COVERAGE_OUT := coverage.out
COVERAGE_HTML:= coverage.html
MIN_COVERAGE := 80
MIGRATE_DIR  := migrations
DATABASE_URL ?= $(shell echo $$DATABASE_URL)
DOCKER_IMAGE := loghunter

# ─── Run ─────────────────────────────────────────────────────────────────────

run: ## Start server locally
	go run ./cmd/server

# ─── Test ────────────────────────────────────────────────────────────────────

test: ## Run all tests with race detector
	go test -race -count=1 ./...

test-unit: ## Run unit tests only (no testcontainers)
	go test -race -count=1 -short ./...

test-integration: ## Run integration tests only
	go test -race -count=1 -run 'Integration|Testcontainers' ./...

# ─── Lint ────────────────────────────────────────────────────────────────────

lint: ## Run golangci-lint
	golangci-lint run ./...

# ─── Migrations ──────────────────────────────────────────────────────────────

migrate-up: ## Apply all migrations
	migrate -path $(MIGRATE_DIR) -database "$(DATABASE_URL)" up

migrate-down: ## Revert last migration
	migrate -path $(MIGRATE_DIR) -database "$(DATABASE_URL)" -verbose down 1

# ─── Build ───────────────────────────────────────────────────────────────────

build: $(SERVER_BIN) $(CLI_BIN) ## Compile server + CLI binaries to bin/

$(SERVER_BIN):
	@mkdir -p $(BIN_DIR)
	go build -o $(SERVER_BIN) ./cmd/server

$(CLI_BIN):
	@mkdir -p $(BIN_DIR)
	go build -o $(CLI_BIN) ./cmd/cli

# ─── Docker ──────────────────────────────────────────────────────────────────

docker-build: ## Build Docker image
	docker build -t $(DOCKER_IMAGE) .

# ─── Coverage ────────────────────────────────────────────────────────────────

coverage: ## Generate coverage.html report; fail if < 80%
	go test -race -count=1 -coverprofile=$(COVERAGE_OUT) ./...
	go tool cover -html=$(COVERAGE_OUT) -o $(COVERAGE_HTML)
	@TOTAL=$$(go tool cover -func=$(COVERAGE_OUT) | grep '^total:' | awk '{print $$NF}' | sed 's/%//'); \
	echo "Total coverage: $${TOTAL}%"; \
	if [ $$(echo "$${TOTAL} < $(MIN_COVERAGE)" | bc -l 2>/dev/null || echo 1) -eq 1 ] && [ "$${TOTAL}" != "" ]; then \
		WHOLE=$$(echo "$${TOTAL}" | cut -d. -f1); \
		if [ "$$WHOLE" -lt "$(MIN_COVERAGE)" ]; then \
			echo "FAIL: coverage $${TOTAL}% is below minimum $(MIN_COVERAGE)%"; \
			exit 1; \
		fi; \
	fi; \
	echo "OK: coverage $${TOTAL}% meets minimum $(MIN_COVERAGE)%"

# ─── Clean ───────────────────────────────────────────────────────────────────

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) $(COVERAGE_OUT) $(COVERAGE_HTML)

# ─── Help ────────────────────────────────────────────────────────────────────

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
