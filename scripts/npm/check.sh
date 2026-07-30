#!/usr/bin/env bash
set -euo pipefail

SCRIPT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ROOT="${SOURCE_ROOT:-$SCRIPT_ROOT}"
cd "$ROOT"

package_dir="${1:-${NPM_PACKAGE_DIR:-$ROOT/.tmp/npm-package}}"
if [[ ! -f "$package_dir/package.json" || ! -f "$package_dir/bin/openapi-sdkgen.js" ]]; then
  echo "npm package directory is incomplete: $package_dir" >&2
  exit 2
fi

package_version="$(node -e 'const fs = require("node:fs"); process.stdout.write(JSON.parse(fs.readFileSync(process.argv[1], "utf8")).version)' "$package_dir/package.json")"
check_cli() {
  local expected_version="$1"
  shift
  local capture_dir
  capture_dir="$(mktemp -d "${TMPDIR:-/tmp}/openapi-sdkgen-cli-check.XXXXXX")"
  local root_help
  local generate_help
  local actual_version
  "$@" --help >"$capture_dir/stdout" 2>"$capture_dir/stderr"
  root_help="$(<"$capture_dir/stdout")"
  if [[ -s "$capture_dir/stderr" ||
    "$root_help" != *$'Usage:\n  openapi-sdkgen <command> [options]'* ||
    "$root_help" != *$'Commands:\n  generate  Generate SDK source'* ]]; then
    echo "openapi-sdkgen root help mismatch" >&2
    rm -rf "$capture_dir"
    exit 1
  fi
  "$@" generate --help >"$capture_dir/stdout" 2>"$capture_dir/stderr"
  generate_help="$(<"$capture_dir/stdout")"
  if [[ -s "$capture_dir/stderr" ||
    "$generate_help" != *$'Usage:\n  openapi-sdkgen generate [options]'* ||
    "$generate_help" != *$'Required:\n  --input <source>'* ]]; then
    echo "openapi-sdkgen generate help mismatch" >&2
    rm -rf "$capture_dir"
    exit 1
  fi
  actual_version="$("$@" --version 2>"$capture_dir/stderr")"
  if [[ "$actual_version" != "openapi-sdkgen $expected_version" ]]; then
    echo "openapi-sdkgen version mismatch: $actual_version" >&2
    rm -rf "$capture_dir"
    exit 1
  fi
  if [[ -s "$capture_dir/stderr" ]]; then
    echo "openapi-sdkgen wrote unexpected stderr" >&2
    rm -rf "$capture_dir"
    exit 1
  fi
  rm -rf "$capture_dir"
}

check_cli "$package_version" node "$package_dir/bin/openapi-sdkgen.js"
test_dir="${NPM_TEST_DIR:-$ROOT/.tmp/npm-package-install}"
rm -rf "$test_dir"
mkdir -p "$test_dir"
npm pack --ignore-scripts --pack-destination "$test_dir" "$package_dir"
tarball="$(find "$test_dir" -maxdepth 1 -name '*.tgz' -print -quit)"
if [[ -z "$tarball" ]]; then
  echo "npm package tarball was not created" >&2
  exit 1
fi
tarball_files="$test_dir/tarball-files.txt"
tar -tzf "$tarball" >"$tarball_files"
required_files=(
  package/package.json
  package/README.md
  package/LICENSE
  package/NOTICE
  package/bin/openapi-sdkgen.js
)
for target in darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64 windows-arm64; do
  executable="openapi-sdkgen"
  if [[ "$target" == windows-* ]]; then
    executable+=".exe"
  fi
  binary="package/bin/$target/$executable"
  required_files+=("$binary")
  go run "$SCRIPT_ROOT/scripts/npm/verify-binary.go" "$target" "$package_dir/bin/$target/$executable"
done
for required_file in "${required_files[@]}"; do
  if ! grep -Fqx "$required_file" "$tarball_files"; then
    echo "npm package tarball is missing required file: $required_file" >&2
    exit 1
  fi
done
install_dir="$test_dir/install"
npm install --ignore-scripts --no-audit --no-fund --offline --prefix "$install_dir" "$tarball"
check_cli "$package_version" "$install_dir/node_modules/.bin/openapi-sdkgen"
echo "ok npm package check"
