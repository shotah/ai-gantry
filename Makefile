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

VERSION ?= $(shell git describe --tags --always --dirty 2>$(NULL) || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>$(NULL) || echo none)

LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: help
help: ## Show available targets
	@echo.
	@echo ai-gantry targets:
	@echo   make build          Build gantry into ./bin
	@echo   make run            Run with CHANNEL=stdio (override: CHANNEL= PERSONA_DIR=)
	@echo   make test           Run all tests
	@echo   make test-verbose   Run tests with -v
	@echo   make race           Race detector (needs CGO)
	@echo   make coverage       Write coverage.out + func summary
	@echo   make coverage-html  HTML report -^> coverage.html
	@echo   make vet            go vet ./...
	@echo   make lint           golangci-lint run ./...
	@echo   make fmt            gofmt (and goimports if installed)
	@echo   make tidy           go mod tidy
	@echo   make ci             tidy fmt vet lint test build
	@echo   make docker-build   Build image gantry:local
	@echo   make docker-stdio   Interactive stdio via compose
	@echo   make version        Print version from built binary
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
coverage: ## Write coverage.out and print function totals
	go test ./... -coverprofile=$(COVERAGE) -covermode=atomic
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
fmt: ## Format with gofmt (+ goimports when available)
	go fmt ./...
ifeq ($(OS),Windows_NT)
	@where goimports >$(NULL) 2>&1 && goimports -w . || echo goimports not installed - skipped
else
	@command -v goimports >/dev/null 2>&1 && goimports -w . || true
endif

.PHONY: tidy
tidy: ## Sync go.mod / go.sum
	go mod tidy

.PHONY: ci
ci: tidy fmt vet lint test build ## Local stand-in for CI checks

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
version: build ## Print gantry version from the built binary
	"$(BINARY)" version

.PHONY: clean
clean: ## Remove build and coverage artifacts
	-$(RM_BIN)
	-$(RM_COV)
	go clean

.DEFAULT_GOAL := help
