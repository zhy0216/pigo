BINARY  = pigo
MODULE  = $(shell go list -m)
VERSION = $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -s -w -X main.Version=$(VERSION)

.PHONY: build run clean test test-race test-cover lint fmt vet prof-mem help

## Build & Run

build: ## Build the binary
	go build -ldflags '$(LDFLAGS)' -o $(BINARY) .

run: ## Run from source
	go run .

clean: ## Remove build artifacts and profiles
	rm -f $(BINARY) *.prof coverage.out

## Testing

test: ## Run tests
	go test ./...

test-race: ## Run tests with race detector
	go test -race ./...

test-cover: ## Run tests with coverage report
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@echo ""
	@echo "To view in browser: go tool cover -html=coverage.out"

## Code Quality

lint: fmt vet ## Run all linters (fmt + vet)

fmt: ## Check formatting
	@test -z "$$(gofmt -l .)" || (gofmt -d . && exit 1)

vet: ## Run go vet
	go vet ./...

## Profiling

prof-mem: build ## Run with memory profiling, then open the profile
	PIGO_MEMPROFILE=mem.prof ./$(BINARY)
	go tool pprof -http=:8080 mem.prof

## Help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
