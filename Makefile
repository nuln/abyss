.PHONY: all help test build pro clean deps lint vulnerability-check

# Default target
all: build

# Environment variables for private modules
export GOPRIVATE=github.com/nuln/*
export GONOSUMDB=github.com/nuln/*

# Versions
GOLANGCI_LINT_VERSION ?= latest
GOVULNCHECK_VERSION ?= latest

help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  deps                Clean go.sum and update dependencies"
	@echo "  build               Build abyss (normal version)"
	@echo "  pro                 Build abyss (pro version)"
	@echo "  test                Run tests"
	@echo "  lint                Run golangci-lint"
	@echo "  vulnerability-check Run govulncheck for dependency scanning"
	@echo "  clean               Remove build artifacts and clear Go cache"

deps:
	@echo "Updating dependencies..."
	@rm -f go.sum
	@go mod tidy
	@go mod download

vulnerability-check:
	@echo "Running govulncheck..."
	@go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	@govulncheck ./...

lint:
	@echo "Running golangci-lint..."
	@golangci-lint run

build:
	@echo "Building abyss..."
	@go build -ldflags="-s -w" -o abyss .

pro:
	@echo "Building abyss-pro..."
	@go build -tags pro -ldflags="-s -w" -o abyss-pro .

test:
	@echo "Running tests..."
	@go test ./...

clean:
	@echo "Cleaning..."
	@rm -f abyss abyss-pro
	@go clean -cache
	@go clean -modcache
