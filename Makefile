.DEFAULT_GOAL := help

SHELL := /usr/bin/env bash

GO_CACHE_DIR := .cache/go-build
GO_TMP_DIR := .cache/go-tmp
GO_ENV := GOCACHE=$$(pwd)/$(GO_CACHE_DIR) GOTMPDIR=$$(pwd)/$(GO_TMP_DIR)

GORELEASER ?= goreleaser

.PHONY: help ensure-cache build doctor test race vet smoke release-check release-snapshot

## Show this help message
help:
	@awk '\
	  BEGIN {FS = ":"} \
	  /^### / {section=substr($$0,5); next} \
	  /^##/ {sub(/^## ?/, "", $$0); helpMsg = $$0; next} \
	  /^[a-zA-Z0-9_.-]+:/ { \
	    sub(/:.*/, "", $$1); \
	    if (helpMsg) { \
	      if (section) { \
	        printf "\n\033[1m%s\033[0m\n", section; \
	        section = ""; \
	      } \
	      printf "  \033[36m%-20s\033[0m %s\n", $$1, helpMsg; \
	      helpMsg = ""; \
	    } \
	  }' $(MAKEFILE_LIST)

ensure-cache:
	@mkdir -p $(GO_CACHE_DIR) $(GO_TMP_DIR)

### Build & Quality
## Build the lazy-django binary
build:
	./build.sh

## Run dependency doctor in strict mode against demo-project
doctor: ensure-cache
	$(GO_ENV) go run ./cmd/lazy-django --doctor --doctor-strict --project ./demo-project

## Run tests
test: ensure-cache
	$(GO_ENV) go test ./...

## Run tests with race detector
race: ensure-cache
	$(GO_ENV) go test -race ./...

## Run go vet
vet: ensure-cache
	$(GO_ENV) go vet ./...

## Run release-readiness smoke test
smoke:
	./smoke-test.sh

### Release
## Validate GoReleaser configuration
release-check: ensure-cache
	@command -v $(GORELEASER) >/dev/null 2>&1 || { echo "Error: goreleaser not found. Install with: brew install goreleaser"; exit 1; }
	$(GO_ENV) $(GORELEASER) check

## Build snapshot artifacts locally (no publish)
release-snapshot: ensure-cache
	@command -v $(GORELEASER) >/dev/null 2>&1 || { echo "Error: goreleaser not found. Install with: brew install goreleaser"; exit 1; }
	$(GO_ENV) $(GORELEASER) release --snapshot --clean --skip=publish --skip=announce --skip=homebrew
