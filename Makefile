# situs Makefile — every standard task for development and CI.
.PHONY: all build run test test-coverage pipeline-test lint vet fmt fmt-check arch debt \
        debt-guard debt-coverage mutation print-gremlins-version verify docs docs-serve \
        hooks security vuln licenses release-dry help

BINARY_NAME := situs
MODULE      := github.com/jobrunner/situs
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS     := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

GO       := go
GOLINT   := golangci-lint
# Pinned in one place; the Mutation workflow reads this same value.
GREMLINS_VERSION  := v0.6.0
CCSH_VERSION      := 1.143.0
GCOV2LCOV_VERSION := v1.1.1
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

# The pipeline produces every fact in the index, including the header-drift
# guard that exists because silently dropping a whole sheet branch already
# happened once. Stdlib-only python3, so there is nothing to install.
pipeline-test: ## Run the XLSX->CSV pipeline's Python tests
	cd pipelines/eunis && python3 -m unittest discover

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

codecharta: ## CodeCharta map (structure+complexity+coverage+git) -> situs.cc.json.gz, then the ratchet (needs node+java)
	@command -v ccsh >/dev/null 2>&1 || npm install -g codecharta-analysis@$(CCSH_VERSION)
	$(GO) install github.com/jandelgado/gcov2lcov@$(GCOV2LCOV_VERSION)
	ccsh unifiedparser . -fe=go -e='_test\.go,third_party' -nc -o base.cc.json
	ccsh gitlogparser repo-scan --repo-path=. --add-author --silent -nc -o git.cc.json
	@$(GO) test -coverprofile=coverage.out ./... >/dev/null || true; \
	 gobin=$$($(GO) env GOBIN); [ -n "$$gobin" ] || gobin=$$($(GO) env GOPATH)/bin; \
	 inputs="base.cc.json git.cc.json"; \
	 if [ -s coverage.out ]; then \
	   "$$gobin"/gcov2lcov -infile=coverage.out -outfile=coverage.info; \
	   ccsh coverageimport coverage.info -f lcov -nc -o coverage.cc.json; \
	   inputs="$$inputs coverage.cc.json"; \
	 else echo "WARN: no coverage.out — map without coverage"; fi; \
	 ccsh merge $$inputs -o situs.cc.json.gz
	python3 scripts/codecharta-ratchet.py situs.cc.json.gz .codecharta-ratchet.json
	@echo "-> situs.cc.json.gz  (load in https://maibornwolff.github.io/codecharta/visualization/)"

print-gremlins-version: ## Echo the pinned gremlins version (used by CI)
	@echo $(GREMLINS_VERSION)

mutation: ## Per-package mutation thresholds (runs locally, macOS included)
	# Installed unconditionally and invoked by absolute path: skipping the install
	# when any gremlins is on PATH would silently run a different engine than CI,
	# and the binary self-reports its version as "dev", so it cannot be checked.
	@$(GO) install github.com/go-gremlins/gremlins/cmd/gremlins@$(GREMLINS_VERSION)
	@gobin=$$($(GO) env GOBIN); [ -n "$$gobin" ] || gobin=$$($(GO) env GOPATH)/bin; \
	 GREMLINS="$$gobin/gremlins" ./scripts/mutation-gate.sh

## Canonical, non-mutating "is it green?" — mirrored in CI.
verify: fmt-check vet lint test pipeline-test arch debt ## Authoritative green check
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
