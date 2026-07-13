#!/usr/bin/env bash
set -euo pipefail

SCRIPT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ROOT="${SOURCE_ROOT:-$SCRIPT_ROOT}"
WORK_ROOT="${RELEASE_CHECK_DIR:-$ROOT/.tmp/release-check}"
TYPESCRIPT_ROOT="$ROOT/test/typescript"
PNPM_VERSION="11.11.0"

if [[ $# -ne 1 ]]; then
  echo "usage: scripts/release/check.sh <semver>" >&2
  exit 2
fi

cd "$ROOT"
if ! command -v corepack >/dev/null 2>&1; then
  echo "release checks require Corepack (Node.js 24)" >&2
  exit 1
fi

rm -rf "$WORK_ROOT"
mkdir -p "$WORK_ROOT/go-build" "$WORK_ROOT/go-mod" "$WORK_ROOT/pnpm-store" "$WORK_ROOT/corepack" "$WORK_ROOT/node-cache"
chmod 700 "$WORK_ROOT" "$WORK_ROOT"/*

export GOCACHE="$WORK_ROOT/go-build"
export GOMODCACHE="$WORK_ROOT/go-mod"
export COREPACK_HOME="$WORK_ROOT/corepack"
export npm_config_cache="$WORK_ROOT/node-cache"
export CI=true

go_files=()
while IFS= read -r -d '' file; do
  go_files+=("$file")
done < <(git ls-files -co --exclude-standard -z -- '*.go')
if ((${#go_files[@]} > 0)); then
  unformatted="$(gofmt -l "${go_files[@]}")"
  if [[ -n "$unformatted" ]]; then
    printf 'release checks found unformatted Go files:\n%s\n' "$unformatted" >&2
    exit 1
  fi
fi

go mod tidy -diff
go mod verify
go vet ./...
go test ./...
go build -o "$WORK_ROOT/openapi-sdkgen" ./cmd/openapi-sdkgen

corepack "pnpm@$PNPM_VERSION" --dir "$TYPESCRIPT_ROOT" --config.store-dir="$WORK_ROOT/pnpm-store" install --frozen-lockfile
corepack "pnpm@$PNPM_VERSION" --dir "$TYPESCRIPT_ROOT" --config.store-dir="$WORK_ROOT/pnpm-store" run fmt:check
corepack "pnpm@$PNPM_VERSION" --dir "$TYPESCRIPT_ROOT" --config.store-dir="$WORK_ROOT/pnpm-store" run lint

fixture="$TYPESCRIPT_ROOT/fixtures/generated/client"
rm -rf "$fixture"
"$WORK_ROOT/openapi-sdkgen" generate \
  --input "$TYPESCRIPT_ROOT/fixtures/contract.openapi.json" \
  --target typescript \
  --output "$fixture"
corepack "pnpm@$PNPM_VERSION" --dir "$TYPESCRIPT_ROOT" --config.store-dir="$WORK_ROOT/pnpm-store" run conformance
corepack "pnpm@$PNPM_VERSION" --dir "$TYPESCRIPT_ROOT" --config.store-dir="$WORK_ROOT/pnpm-store" run coverage

SOURCE_ROOT="$ROOT" \
  NPM_PACKAGE_DIR="$WORK_ROOT/npm-package" \
  bash "$SCRIPT_ROOT/scripts/npm/package.sh" "$1"
SOURCE_ROOT="$ROOT" \
  NPM_PACKAGE_DIR="$WORK_ROOT/npm-package" \
  NPM_TEST_DIR="$WORK_ROOT/npm-package-install" \
  bash "$SCRIPT_ROOT/scripts/npm/check.sh" "$WORK_ROOT/npm-package"

printf 'ok release checks %s\n' "$1"
