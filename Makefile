BINARY := bin/gomailtest
ifeq ($(OS),Windows_NT)
  BINARY := bin/gomailtest.exe
endif

VERSION := $(shell grep -oP 'Version = "\K[^"]+' internal/common/version/version.go 2>/dev/null || echo unknown)

.PHONY: build build-verbose test integration-test clean help

build: ## Build the gomailtest binary
	go build -ldflags="-s -w" -o $(BINARY) ./cmd/gomailtest
	@echo "Built $(BINARY) — version $(VERSION)"

build-verbose: ## Build the gomailtest binary with verbose output
	go build -v -ldflags="-s -w" -o $(BINARY) ./cmd/gomailtest
	@echo "Built $(BINARY) — version $(VERSION)"

test: ## Run unit tests
	go test ./...

integration-test: build ## Run MS Graph integration tests (requires MSGRAPH* env vars)
	@sh scripts/check-integration-env.sh
	go test -tags integration -v -timeout 120s ./tests/integration/

clean: ## Remove build artifacts
	rm -f $(BINARY)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*##"}; {printf "  %-20s %s\n", $$1, $$2}'
