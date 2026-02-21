#!/usr/bin/env bash
# Build script for lazy-django

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

echo "Building lazy-django..."

# Check if Go is installed
if ! command -v go >/dev/null 2>&1; then
	echo "Error: Go is not installed"
	echo "Install Go from: https://go.dev/doc/install"
	echo ""
	echo "On macOS with Homebrew:"
	echo "  brew install go"
	exit 1
fi

# Keep build artifacts/cache local to repo for portability.
export GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}"
export GOTMPDIR="${GOTMPDIR:-$ROOT_DIR/.cache/go-tmp}"
mkdir -p "$GOCACHE" "$GOTMPDIR"

echo "Downloading dependencies..."
go mod download

echo "Building binary..."
go build -o lazy-django ./cmd/lazy-django

echo "Done! Run './lazy-django' in a Django project directory"
