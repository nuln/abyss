# Abyss Makefile
#
# Usage:
#   make setup      Initialize environment and submodules
#   make build      Build the abyss binary
#   make test       Run all tests
#   make lint       Run linter
#   make fmt        Format code
#   make coverage   Run coverage report

.PHONY: all help setup build example ui-build ui-sync test coverage scan lint fmt clean

all: setup fmt ui-build lint scan test build coverage

# --- Variables ---
BINARY_NAME=abyss-core
UI_DIR=www
UI_REPO=git@github.com:nuln/abyss-www.git

help:
	@echo "Available targets:"
	@echo "  setup    - Initialize git submodules and install dependencies"
	@echo "  build    - Sync UI, build UI (via www/Makefile), and build Go binary"
	@echo "  example  - Build Base version binary (no Pro plugins)"
	@echo "  example-pro - Build Pro version binary (all plugins)"
	@echo "  test     - Run Go tests"
	@echo "  coverage - Run Go coverage report"
	@echo "  lint     - Run Go linter"
	@echo "  fmt      - Format Go code"
	@echo "  clean    - Remove build artifacts"

# --- Setup & Sync ---

setup: ui-sync
	@echo "Installing Go dependencies..."
	go mod tidy
	@echo "Calling UI setup..."
	@$(MAKE) -C $(UI_DIR) setup --no-print-directory

ui-sync:
	@echo "Syncing UI submodule..."
	@if [ ! -d "$(UI_DIR)/.git" ]; then \
		git submodule add $(UI_REPO) $(UI_DIR) || git submodule update --init --recursive; \
	else \
		git submodule update --remote --merge; \
	fi

# --- Build ---

build: ui-build
	@echo "Verifying Go compilation..."
	go build ./...

ui-build:
	@echo "Delegating UI build to $(UI_DIR)/Makefile..."
	@$(MAKE) -C $(UI_DIR) build --no-print-directory

example: ui-build plugins-build
	@echo "Building abyss binary (Base)..."
	(cd example && go build -o abyss main.go)

example-pro: ui-build plugins-build
	@echo "Building abyss binary (Pro)..."
	(cd example/pro && go build -o ../abyss main.go)

plugins-build:
	@echo "Building plugin frontends..."
	@for dir in plugins/* pro/*; do \
		if [ -d "$$dir" ] && [ -f "$$dir/Makefile" ]; then \
			echo "Building frontend for $$dir..."; \
			$(MAKE) -C $$dir build-frontend --no-print-directory || exit 1; \
		fi \
	done

# --- Development ---

test:
	@echo "Running tests..."
	go test -v ./...

coverage:
	@echo "Running coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

scan:
	@echo "Running vulnerability check..."
	govulncheck ./...

lint:
	@echo "Running linter..."
	golangci-lint run ./...
	@echo "Delegating UI lint to $(UI_DIR)/Makefile..."
	@$(MAKE) -C $(UI_DIR) lint --no-print-directory

fmt:
	@echo "Formatting code..."
	go fmt ./...
	@echo "Delegating UI fmt to $(UI_DIR)/Makefile..."
	@$(MAKE) -C $(UI_DIR) fmt --no-print-directory

clean:
	@echo "Cleaning up..."
	rm -f coverage.out
	@echo "Delegating UI clean to $(UI_DIR)/Makefile..."
	@$(MAKE) -C $(UI_DIR) clean --no-print-directory
