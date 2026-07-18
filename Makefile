BINARY := bin/gomailtest
ifeq ($(OS),Windows_NT)
  BINARY := bin/gomailtest.exe
endif

VERSION := $(shell grep -oP 'Version = "\K[^"]+' internal/common/version/version.go 2>/dev/null || echo unknown)

# All known protocol tags (used as default for build-custom)
ALL_PROTOCOLS := smtp imap pop3 jmap ews gmail msgraph

# For protocol-subset builds: override on the command line, e.g.
#   make build-custom PROTOCOLS="smtp imap pop3"
PROTOCOLS ?= $(ALL_PROTOCOLS)
PROTO_SUFFIX := $(shell echo $(PROTOCOLS) | tr ' ' '-')
ifeq ($(OS),Windows_NT)
  BINARY_CUSTOM := bin/gomailtest-$(PROTO_SUFFIX).exe
else
  BINARY_CUSTOM := bin/gomailtest-$(PROTO_SUFFIX)
endif

.PHONY: build build-verbose build-custom build-smtp-only build-msgraph-only build-standard-only test integration-test clean help

build: ## Build the gomailtest binary with all protocols (optimized for size & reproducibility)
	go build -ldflags="-s -w" -trimpath -o $(BINARY) ./cmd/gomailtest
	@echo "Built $(BINARY) — version $(VERSION)"

build-verbose: ## Build the gomailtest binary with verbose output
	go build -v -ldflags="-s -w" -trimpath -o $(BINARY) ./cmd/gomailtest
	@echo "Built $(BINARY) — version $(VERSION)"

build-custom: ## Build a subset binary; set PROTOCOLS="smtp imap ..." to choose protocols
	go build -tags "custom $(PROTOCOLS)" -ldflags="-s -w" -trimpath -o $(BINARY_CUSTOM) ./cmd/gomailtest
	@echo "Built $(BINARY_CUSTOM) — protocols: $(PROTOCOLS) — version $(VERSION)"

build-smtp-only: ## Build a binary with SMTP protocol only
	$(MAKE) build-custom PROTOCOLS=smtp

build-msgraph-only: ## Build a binary with Microsoft Graph protocol only
	$(MAKE) build-custom PROTOCOLS=msgraph

build-standard-only: ## Build a binary with SMTP, IMAP, and POP3 protocols only
	$(MAKE) build-custom PROTOCOLS="smtp imap pop3"

test: ## Run unit tests
	go test ./...

integration-test: build ## Run MS Graph integration tests (requires MSGRAPH* env vars)
	@sh scripts/check-integration-env.sh
	go test -tags integration -v -timeout 120s ./tests/integration/

clean: ## Remove build artifacts
	rm -f $(BINARY) bin/gomailtest-*

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*##"}; {printf "  %-20s %s\n", $$1, $$2}'
