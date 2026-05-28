# Abyss UI Makefile
#
# This Makefile is used to manage the frontend build process.

.PHONY: all help setup build dev lint fmt clean

all: setup fmt lint build

help:
	@echo "Available UI targets:"
	@echo "  setup    - Install frontend dependencies"
	@echo "  build    - Build production assets"
	@echo "  dev      - Start development server"
	@echo "  lint     - Run ESLint"
	@echo "  fmt      - Format code using ESLint fix"
	@echo "  clean    - Remove build artifacts"

setup:
	@echo "Installing pnpm dependencies..."
	@pnpm install

DIST_MARKER := dist/index.html
SRC_FILES := $(shell find src public -type f 2>/dev/null) package.json pnpm-lock.yaml vite.config.ts index.html

$(DIST_MARKER): $(SRC_FILES)
	@echo "Changes detected in frontend source. Building assets..."
	@pnpm build
	touch $(DIST_MARKER)

build: $(DIST_MARKER)
	@echo "Frontend is up to date."

dev:
	@echo "Starting dev server..."
	@pnpm dev

lint:
	@echo "Linting frontend..."
	@pnpm lint

fmt:
	@echo "Formatting frontend..."
	@pnpm lint --fix

scan:
	@echo "Running security audit..."
	@pnpm audit --audit-level=high

package: build
	@echo "Packaging assets..."
	@cd dist && zip -r ../abyss-ui-assets.zip .

clean:
	@echo "Cleaning artifacts..."
	@rm -rf dist abyss-ui-assets.zip
	mkdir -p dist
	touch dist/.gitkeep
