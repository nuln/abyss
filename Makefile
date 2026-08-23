# Abyss Makefile
#
# Usage:
#   make setup      Initialize environment and install dependencies
#   make build      Build the abyss binary
#   make test       Run all tests
#   make lint       Run linter
#   make fmt        Format code
#   make coverage   Run coverage report

.PHONY: all help setup build example ui-build test coverage scan lint fmt clean

all: setup fmt ui-build lint test build coverage

# --- Variables ---
BINARY_NAME=abyss
UI_DIR=www

help:
	@echo "Available targets:"
	@echo "  setup    - Install Go and frontend dependencies"
	@echo "  build    - Build UI (via www/Makefile), and build Go binary"
	@echo "  example  - Build Base version binary (no Pro plugins)"
	@echo "  example-pro - Build Pro version binary (all plugins)"
	@echo "  test     - Run Go tests"
	@echo "  coverage - Run Go coverage report"
	@echo "  lint     - Run Go linter"
	@echo "  fmt      - Format code"
	@echo "  clean    - Remove build artifacts"

# --- Setup ---

setup:
	@echo "Installing Go dependencies..."
	go mod tidy
	@echo "Calling UI setup..."
	@$(MAKE) -C $(UI_DIR) setup --no-print-directory

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
	rm -rf dist
	@echo "Delegating UI clean to $(UI_DIR)/Makefile..."
	@$(MAKE) -C $(UI_DIR) clean --no-print-directory

# --- Release ---
#
# release-build  Cross-compile the standalone server (cmd/abyss) for all
#                release platforms into dist/release/.
# release-assets Build everything and publish a GitHub release. Invoked by
#                CI on tag push via the shared core-assets action:
#                    make release-assets TAG=v1.2.3

.PHONY: release-build release-assets

RELEASE_PLATFORMS := linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64

release-build:
	@if [ ! -d "$(UI_DIR)/node_modules" ]; then \
		echo "Installing frontend dependencies..."; \
		cd $(UI_DIR) && pnpm install --frozen-lockfile --silent; \
	fi
	@$(MAKE) ui-build
	@echo "Cross-compiling release binaries ($(RELEASE_PLATFORMS))..."
	@mkdir -p dist/release
	@for platform in $(RELEASE_PLATFORMS); do \
		os=$${platform%%-*}; arch=$${platform##*-}; \
		ext=; archive=tar.gz; \
		if [ "$$os" = "windows" ]; then ext=.exe; archive=zip; fi; \
		name=abyss-$(TAG)-$$platform; \
		echo "  building $$name"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
			go build -trimpath -ldflags "-s -w" \
			-o dist/release/$$name$$ext ./cmd/abyss || exit 1; \
		( cd dist/release && \
			if [ "$$archive" = "zip" ]; then zip -q $$name.zip $$name$$ext; \
			else tar czf $$name.tar.gz $$name$$ext; fi; \
			rm -f $$name$$ext ); \
	done
	@echo "Release artifacts ready in dist/release/"

# NOTE: the shared core-assets action injects the repo token only into the
# git remote URL (not the environment); recover it from there for gh.
release-assets: release-build
	@if [ -z "$(TAG)" ]; then echo "usage: make release-assets TAG=v1.2.3" >&2; exit 2; fi; \
	if [ -n "$$GITHUB_TOKEN" ]; then \
		export GH_TOKEN="$$GITHUB_TOKEN"; \
	else \
		tok=$$(git config --get remote.origin.url | sed -n 's|^https://x-access-token:\([^@]*\)@.*$$|\1|p'); \
		if [ -n "$$tok" ]; then export GH_TOKEN="$$tok"; fi; \
	fi; \
	gh release create "$(TAG)" --title "$(TAG)" --generate-notes dist/release/*
