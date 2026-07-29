#!/usr/bin/env bash
set -euo pipefail

SCRIPT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ROOT="${SOURCE_ROOT:-$SCRIPT_ROOT}"
WORK_ROOT="${RELEASE_CHECK_DIR:-$ROOT/.tmp/release-check}"
TYPESCRIPT_ROOT="$ROOT/test/typescript"
PNPM_VERSION="11.11.0"
LOG_ROOT="$WORK_ROOT/logs"
GO_BUILD_CACHE="${GOCACHE:-$(go env GOCACHE)}"
GO_MODULE_CACHE="${GOMODCACHE:-$(go env GOMODCACHE)}"
COREPACK_CACHE="${COREPACK_HOME:-$ROOT/.tmp/corepack}"
NODE_CACHE="${npm_config_cache:-$ROOT/.tmp/node-cache}"
PNPM_STORE="${npm_config_store_dir:-$ROOT/.tmp/pnpm-store}"

if [[ $# -ne 1 ]]; then
  echo "usage: scripts/release/check.sh <semver>" >&2
  exit 2
fi

cd "$ROOT"
if ! command -v corepack >/dev/null 2>&1; then
  echo "release checks require Corepack (Node.js 24)" >&2
  exit 1
fi

# Agent wrappers provide repository-local caches. Direct and CI runs use Go's
# standard cache paths, which lets actions/setup-go restore them between runs.
mkdir -p "$WORK_ROOT" "$LOG_ROOT" "$GO_BUILD_CACHE" "$GO_MODULE_CACHE" "$COREPACK_CACHE" "$NODE_CACHE" "$PNPM_STORE"
chmod 700 "$WORK_ROOT" "$LOG_ROOT"

export GOCACHE="$GO_BUILD_CACHE"
export GOMODCACHE="$GO_MODULE_CACHE"
export COREPACK_HOME="$COREPACK_CACHE"
export npm_config_cache="$NODE_CACHE"
export CI=true

run_step() {
  local label="$1"
  shift
  local safe_label
  safe_label="$(printf '%s' "$label" | tr -cs 'A-Za-z0-9_.-' '-')"
  local log_file="$LOG_ROOT/${safe_label}.log"
  local started="$SECONDS"

  printf 'run %s\n' "$label"
  if "$@" >"$log_file" 2>&1; then
    printf 'ok %s (%ss)\n' "$label" "$((SECONDS - started))"
    return 0
  else
    local status=$?
    printf 'failed %s (%ss)\n' "$label" "$((SECONDS - started))" >&2
    printf 'log: %s\n' "$log_file" >&2
    tail -n 80 "$log_file" >&2 || true
    return "$status"
  fi
}

check_go_format() {
  local go_files=()
  while IFS= read -r -d '' file; do
    go_files+=("$file")
  done < <(git ls-files -co --exclude-standard -z -- '*.go')
  if ((${#go_files[@]} == 0)); then
    return 0
  fi
  local unformatted
  unformatted="$(gofmt -l "${go_files[@]}")"
  if [[ -n "$unformatted" ]]; then
    printf 'release checks found unformatted Go files:\n%s\n' "$unformatted" >&2
    return 1
  fi
}

run_step "Go format" check_go_format
run_step "Go module tidy" go mod tidy -diff
run_step "Go module verify" go mod verify
run_step "Go vet" go vet ./...
run_step "Go test" go test ./...
run_step "Go build" go build -o "$WORK_ROOT/openapi-sdkgen" ./cmd/openapi-sdkgen

run_step "TypeScript install" corepack "pnpm@$PNPM_VERSION" --dir "$TYPESCRIPT_ROOT" --config.store-dir="$PNPM_STORE" install --frozen-lockfile
run_step "TypeScript format" corepack "pnpm@$PNPM_VERSION" --dir "$TYPESCRIPT_ROOT" --config.store-dir="$PNPM_STORE" run fmt:check
run_step "TypeScript lint" corepack "pnpm@$PNPM_VERSION" --dir "$TYPESCRIPT_ROOT" --config.store-dir="$PNPM_STORE" run lint

fixture="$TYPESCRIPT_ROOT/fixtures/generated/client"
rm -rf "$fixture"
run_step "TypeScript fixture" "$WORK_ROOT/openapi-sdkgen" generate \
  --input "$TYPESCRIPT_ROOT/fixtures/contract.openapi.json" \
  --target typescript \
  --output "$fixture"
# Do not delegate through the `conformance` package script here. On a fresh
# GitHub runner, `corepack pnpm` can execute pnpm without installing a global
# `pnpm` shim, while a nested `pnpm run ...` cannot resolve that shim.
run_step "TypeScript typecheck" corepack "pnpm@$PNPM_VERSION" --dir "$TYPESCRIPT_ROOT" --config.store-dir="$PNPM_STORE" run typecheck
# Coverage executes the complete Vitest suite, so a separate test run would
# repeat every TypeScript test without adding release confidence.
run_step "TypeScript test coverage" corepack "pnpm@$PNPM_VERSION" --dir "$TYPESCRIPT_ROOT" --config.store-dir="$PNPM_STORE" run coverage

run_step "npm package" env \
  SOURCE_ROOT="$ROOT" \
  NPM_PACKAGE_DIR="$WORK_ROOT/npm-package" \
  bash "$SCRIPT_ROOT/scripts/npm/package.sh" "$1"
run_step "npm package check" env \
  SOURCE_ROOT="$ROOT" \
  NPM_PACKAGE_DIR="$WORK_ROOT/npm-package" \
  NPM_TEST_DIR="$WORK_ROOT/npm-package-install" \
  bash "$SCRIPT_ROOT/scripts/npm/check.sh" "$WORK_ROOT/npm-package"

printf 'ok release checks %s\n' "$1"
