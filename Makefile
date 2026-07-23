# ai-gantry — common operator commands
# Usage: make <target>

CMD           := ./cmd/gantry
BIN_DIR       := bin
COVERAGE      := coverage.out
COVERAGE_HTML := coverage.html

# Static binary by default (matches container contract).
export CGO_ENABLED ?= 0

CHANNEL     ?= stdio
PERSONA_DIR ?= ./deploy/persona

ifeq ($(OS),Windows_NT)
	BINARY    := $(BIN_DIR)/gantry.exe
	NULL      := NUL
	DATE      ?= unknown
	MKDIR_BIN  = if not exist "$(BIN_DIR)" mkdir "$(BIN_DIR)"
	RM_BIN     = if exist "$(BIN_DIR)" rmdir /s /q "$(BIN_DIR)"
	RM_COV     = if exist "$(COVERAGE)" del /q "$(COVERAGE)" & if exist "$(COVERAGE_HTML)" del /q "$(COVERAGE_HTML)"
	RUN_ENV    = set "CHANNEL=$(CHANNEL)"&& set "PERSONA_DIR=$(PERSONA_DIR)"&&
else
	BINARY    := $(BIN_DIR)/gantry
	NULL      := /dev/null
	DATE      ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
	MKDIR_BIN  = mkdir -p "$(BIN_DIR)"
	RM_BIN     = rm -rf "$(BIN_DIR)"
	RM_COV     = rm -f "$(COVERAGE)" "$(COVERAGE_HTML)"
	RUN_ENV    = CHANNEL="$(CHANNEL)" PERSONA_DIR="$(PERSONA_DIR)"
endif

# Build-time version stamp (git describe). Release tags are tracked in ./VERSION.
VERSION ?= $(shell git describe --tags --always --dirty 2>$(NULL) || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>$(NULL) || echo none)

# Release bump: patch (default), minor, or major. Or set TAG=v0.2.0 explicitly.
BUMP ?= patch

LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: help
help: ## Show available targets
	@echo.
	@echo ai-gantry targets:
	@echo   make build          Build gantry into ./bin
	@echo   make run            Run with CHANNEL=stdio (override: CHANNEL= PERSONA_DIR=)
	@echo   make init           Scaffold deploy/persona + deploy/mcp.toml via gantry init
	@echo   make example-pa     Seed examples/personal-assistant (Tim-shaped compose)
	@echo   make test           Run all tests
	@echo   make test-verbose   Run tests with -v
	@echo   make race           Race detector (needs CGO)
	@echo   make coverage       Write coverage.out + func summary
	@echo   make coverage-html  HTML report -^> coverage.html
	@echo   make vet            go vet ./...
	@echo   make lint           golangci-lint run ./...
	@echo   make fmt            Autofix imports/code (goimports-reviser + golangci-lint)
	@echo   make tidy           go mod tidy
	@echo   make check          Autofix, lint, and test (matches pre-commit)
	@echo   make ci             tidy fmt vet lint test build
	@echo   make docker-build   Build image gantry:local
	@echo   make docker-stdio   Interactive stdio via compose
	@echo   make version        Show VERSION file + next tag (dry-run)
	@echo   make release        Bump tag, update VERSION, push (BUMP=patch^|minor^|major)
	@echo   make install-hooks  Install git pre-commit (autofix + lint + test)
	@echo   make tools          Install goimports-reviser + golangci-lint v2
	@echo   make clean          Remove build/coverage artifacts
	@echo.

.PHONY: all
all: fmt vet lint test build ## Format, vet, lint, test, then build

.PHONY: build
build: ## Build gantry into ./bin
	@$(MKDIR_BIN)
	go build -trimpath -ldflags="$(LDFLAGS)" -o "$(BINARY)" $(CMD)
	@echo built $(BINARY)

.PHONY: run
run: ## Run gantry (CHANNEL=stdio by default for local REPL)
	$(RUN_ENV) go run $(CMD) run

.PHONY: init
init: ## Scaffold deploy/ mounts from embedded examples (gantry init)
	go run $(CMD) init

.PHONY: example-pa
example-pa: ## Seed examples/personal-assistant persona + .env for compose
ifeq ($(OS),Windows_NT)
	set "PERSONA_DIR=examples/personal-assistant/persona"&& set "MCP_MANIFEST=examples/personal-assistant/mcp.toml"&& go run $(CMD) init
	@if not exist "examples\personal-assistant\.env" copy /Y "examples\personal-assistant\.env.example" "examples\personal-assistant\.env"
else
	PERSONA_DIR=examples/personal-assistant/persona MCP_MANIFEST=examples/personal-assistant/mcp.toml go run $(CMD) init
	@test -f examples/personal-assistant/.env || cp examples/personal-assistant/.env.example examples/personal-assistant/.env
endif
	@echo next: edit examples/personal-assistant/.env then
	@echo   docker compose -f examples/personal-assistant/compose.yml up -d --build

.PHONY: test
test: ## Run all tests
	go test ./...

.PHONY: test-verbose
test-verbose: ## Run all tests with -v
	go test -v ./...

.PHONY: race
race: ## Run tests with the race detector (requires CGO)
	CGO_ENABLED=1 go test -race ./...

.PHONY: coverage
coverage: ## Write coverage.out for ./internal/... ./cmd/... ./examples/... (matches CI badge)
	go test ./internal/... ./cmd/... ./examples/... -coverprofile=$(COVERAGE) -covermode=atomic
	go tool cover -func=$(COVERAGE)

.PHONY: coverage-html
coverage-html: coverage ## HTML coverage report
	go tool cover -html=$(COVERAGE) -o $(COVERAGE_HTML)
	@echo wrote $(COVERAGE_HTML)

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: fmt
fmt: ## Autofix imports/code (goimports-reviser + golangci-lint fmt/fix)
	goimports-reviser -format -recursive .
	-golangci-lint fmt ./...
	-golangci-lint run --fix ./...

.PHONY: tidy
tidy: ## Sync go.mod / go.sum
	go mod tidy

.PHONY: check
check: fmt lint test ## Autofix, lint, test (matches pre-commit)

.PHONY: ci
ci: tidy fmt vet lint test build ## Local stand-in for CI checks

.PHONY: install-hooks
install-hooks: ## Install git pre-commit hook (autofix + lint + test)
ifeq ($(OS),Windows_NT)
	copy /Y scripts\pre-commit .git\hooks\pre-commit
else
	cp scripts/pre-commit .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit
endif
	@echo "Installed .git/hooks/pre-commit"

.PHONY: tools
tools: ## Install goimports-reviser + golangci-lint v2 into $$GOBIN
	go install github.com/incu6us/goimports-reviser/v3@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	@echo Installed tools. Ensure GOPATH/bin is on PATH, then: golangci-lint version

.PHONY: docker-build
docker-build: ## Build the container image (gantry:local)
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-t gantry:local .

.PHONY: docker-stdio
docker-stdio: ## Interactive stdio REPL via compose
	docker compose run --rm -it -e CHANNEL=stdio gantry

.PHONY: version
version: ## Show VERSION file and latest git tag / next patch
	@go run ./cmd/release -dry-run

# Bump semver, commit VERSION, annotated-tag, push HEAD + tag (triggers GoReleaser).
# Examples:
#   make release
#   make release BUMP=minor
#   make release BUMP=major
#   make release TAG=v0.2.0
#   make release DRY_RUN=1
.PHONY: release
release: ## Bump version tag, update VERSION, push (BUMP=patch|minor|major)
	go run ./cmd/release \
		$(if $(TAG),-version=$(TAG),-bump=$(BUMP)) \
		$(if $(DRY_RUN),-dry-run,) \
		$(if $(SKIP_PUSH),-skip-push,) \
		$(if $(ALLOW_DIRTY),-allow-dirty,)

.PHONY: clean
clean: ## Remove build and coverage artifacts
	-$(RM_BIN)
	-$(RM_COV)
	go clean

.DEFAULT_GOAL := help
