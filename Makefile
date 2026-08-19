# situs Makefile — every standard task for development and CI.
.PHONY: all build run test test-coverage lint vet fmt fmt-check arch debt \
        debt-guard debt-coverage mutation verify docs docs-serve \
        hooks security vuln licenses release-dry help

BINARY_NAME := situs
MODULE      := github.com/jobrunner/situs
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS     := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

GO       := go
GOLINT   := golangci-lint
COVERAGE_DIR := coverage
MKDOCS   := uvx --with mkdocs-material mkdocs

all: verify build

## Build
build: ## Build the binary
	$(GO) build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/$(BINARY_NAME)

run: build ## Build and run
	./$(BINARY_NAME) serve

## Test
test: ## Run all tests
	$(GO) test ./...

test-coverage: ## Tests with a coverage report
	@mkdir -p $(COVERAGE_DIR)
	$(GO) test -coverprofile=$(COVERAGE_DIR)/coverage.out -covermode=atomic ./...
	$(GO) tool cover -func=$(COVERAGE_DIR)/coverage.out

## Lint / format
lint: ## golangci-lint
	$(GOLINT) run --timeout=5m ./...

vet: ## go vet
	$(GO) vet ./...

fmt: ## Format
	$(GO) fmt ./...
	goimports -w -local $(MODULE) ./cmd ./internal

fmt-check: ## Check formatting without changing anything (CI/hook)
	@unformatted=$$(gofmt -l cmd internal); \
	if [ -n "$$unformatted" ]; then echo "not formatted:"; echo "$$unformatted"; exit 1; fi

## Architecture fitness: import boundaries + module hygiene
arch: ## depguard + gomodguard + go.mod tidiness
	$(GOLINT) run --enable-only depguard,gomodguard_v2 ./...
	$(GO) mod tidy -diff
	@echo "arch ok."

## Debt ratchets
debt: debt-guard debt-coverage ## Suppression budget + coverage floors

debt-guard: ## Fast grep-based ratchet (suppression budget, debt markers)
	@./scripts/debt-guard.sh

debt-coverage: ## Per-package coverage floors (own test run)
	@mkdir -p $(COVERAGE_DIR)
	@$(GO) test -coverprofile=$(COVERAGE_DIR)/coverage.out -covermode=atomic ./... >/dev/null
	@./scripts/coverage-gate.sh $(COVERAGE_DIR)/coverage.out

mutation: ## Mutation testing (ubuntu only — gremlins panics on macOS)
	$(GO) install github.com/go-gremlins/gremlins/cmd/gremlins@v0.5.1
	gremlins unleash --threshold-efficacy 0 --threshold-mcover 0 ./internal/...

## Canonical, non-mutating "is it green?" — mirrored in CI.
verify: fmt-check vet lint test arch debt ## Authoritative green check
	@echo "Compile-check (go build ./...)…"
	@$(GO) build ./...
	@echo "verify passed."

## Security
security: vuln ## All security checks
vuln: ## Known vulnerabilities
	govulncheck ./...
licenses: ## Dependency license compliance
	go-licenses check ./cmd/$(BINARY_NAME) \
		--allowed_licenses=Apache-2.0,MIT,BSD-3-Clause,BSD-2-Clause,ISC,MPL-2.0 --ignore $(MODULE)

## Docs (Diátaxis, MkDocs Material). --strict fails on broken links/nav.
docs: ## Build the docs strictly
	$(MKDOCS) build --strict
docs-serve: ## Serve the docs with live reload
	$(MKDOCS) serve

## Release (dry run; real releases go through release-please + goreleaser in CI)
release-dry: ## goreleaser snapshot
	goreleaser release --snapshot --clean

## Git hooks
hooks: ## Install the pre-commit hook (.githooks)
	git config core.hooksPath .githooks
	@chmod +x .githooks/pre-commit
	@echo "pre-commit hook active."

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'
