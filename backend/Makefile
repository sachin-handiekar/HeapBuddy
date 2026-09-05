BINARY      := heapbuddy
PKG         := github.com/sachin-handiekar/HeapBuddy/cmd
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE        ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -s -w \
	-X $(PKG).version=$(VERSION) \
	-X $(PKG).commit=$(COMMIT) \
	-X $(PKG).date=$(DATE)

FRONTEND := ../frontend
WEBUI    := internal/server/webui

.PHONY: all build build-api-only dist ui install test cover fmt vet lint tidy clean

all: fmt vet test build

build: ui ## Build the single production binary: embeds the SPA, serves UI + API on one port
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

build-api-only: ## Build without the embedded UI (API only; serves a 503 at "/")
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

ui: ## Build the frontend SPA and stage it for embedding
	cd $(FRONTEND) && if command -v bun >/dev/null 2>&1; then bun install && bun run build; else npm install && npm run build; fi
	rm -rf $(WEBUI)/assets $(WEBUI)/_shell.html
	cp -r $(FRONTEND)/dist/client/assets $(WEBUI)/assets
	cp $(FRONTEND)/dist/client/_shell.html $(WEBUI)/_shell.html

dist: build ## Alias for `build` (kept for compatibility)

install: ## Install the binary into GOBIN
	go install -ldflags "$(LDFLAGS)" .

test: ## Run tests with the race detector
	go test -race -count=1 -timeout 120s ./...

cover: ## Run tests and produce an HTML coverage report
	go test -count=1 -timeout 120s -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

fmt: ## Format all Go source
	gofmt -w .

vet: ## Run go vet
	go vet ./...

lint: ## Run golangci-lint (must be installed)
	golangci-lint run

tidy: ## Tidy module dependencies
	go mod tidy

clean: ## Remove build artifacts
	rm -f $(BINARY) $(BINARY).exe coverage.out coverage.html
