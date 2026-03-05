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
	@echo "  build               Build abyss (normal version) with UPX"
	@echo "  pro                 Build abyss (pro version) with UPX"
	@echo "  test                Run tests"
	@echo "  lint                Run golangci-lint"
	@echo "  vulnerability-check Run govulncheck for dependency scanning"
	@echo "  clean               Remove build artifacts"

deps:
	@echo "Updating dependencies..."
	@rm -f go.sum
	@go mod tidy
	@go mod download

vulnerability-check: deps
	@echo "Running govulncheck..."
	@go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	@govulncheck ./...

lint: deps
	@echo "Running golangci-lint..."
	@golangci-lint run

build: deps
	@echo "Building abyss..."
	@go build -ldflags="-s -w" -o abyss .
	@if command -v upx > /dev/null; then \
		echo "Compressing with UPX..."; \
		upx --best abyss; \
	fi

pro: deps
	@echo "Building abyss-pro..."
	@go build -tags pro -ldflags="-s -w" -o abyss-pro .
	@if command -v upx > /dev/null; then \
		echo "Compressing with UPX..."; \
		upx --best abyss-pro; \
	fi

test: deps
	@echo "Running tests..."
	@go test ./...

clean:
	@echo "Cleaning..."
	@rm -f abyss abyss-pro