#!/usr/bin/env bash
# Smoke test for lazy-django release readiness

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

export GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}"
export GOTMPDIR="${GOTMPDIR:-$ROOT_DIR/.cache/go-tmp}"
mkdir -p "$GOCACHE" "$GOTMPDIR"

echo "=== Smoke Test: lazy-django ==="

echo "1) Build binary"
./build.sh
test -x ./lazy-django

echo "2) Run dependency doctor (strict)"
./lazy-django --doctor --doctor-strict --project "$ROOT_DIR/demo-project"

echo "3) Run full test suite"
go test ./...

echo "4) Run race tests"
go test -race ./...

echo "5) Run vet"
go vet ./...

echo "=== Smoke test passed ==="
