#!/usr/bin/env bash
set -euo pipefail

SCRIPT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ROOT="${SOURCE_ROOT:-$SCRIPT_ROOT}"
cd "$ROOT"

if [[ $# -ne 1 ]]; then
  echo "usage: scripts/npm/package.sh <semver>" >&2
  exit 2
fi

version="$1"
semver='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+)(\.[0-9A-Za-z-]+)*)?(\+([0-9A-Za-z-]+)(\.[0-9A-Za-z-]+)*)?$'
if ! [[ "$version" =~ $semver ]]; then
  echo "npm package version must be SemVer without a leading v: $version" >&2
  exit 2
fi

package_dir="${NPM_PACKAGE_DIR:-$ROOT/.tmp/npm-package}"
rm -rf "$package_dir"
mkdir -p "$package_dir"
cp -R "$ROOT/npm/." "$package_dir/"
cp "$ROOT/LICENSE" "$package_dir/LICENSE"
cp "$ROOT/NOTICE" "$package_dir/NOTICE"

sed -i.bak "s/\"version\": \"0.0.0\"/\"version\": \"$version\"/" "$package_dir/package.json"
rm "$package_dir/package.json.bak"

for target in darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64 windows-arm64; do
  os="${target%-*}"
  arch="${target#*-}"
  executable="openapi-sdkgen"
  if [[ "$os" == "windows" ]]; then
    executable+=".exe"
  fi
  output="$package_dir/bin/$target/$executable"
  mkdir -p "$(dirname "$output")"
  env CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags='-s -w' -o "$output" ./cmd/openapi-sdkgen
done

printf 'ok npm package %s\n' "$version"
