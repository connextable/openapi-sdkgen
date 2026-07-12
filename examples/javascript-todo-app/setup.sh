#!/usr/bin/env bash
set -euo pipefail

directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cli="${SDKGEN_BIN:-openapi-sdkgen}"

rm -rf "$directory/src/generated/todo-sdk"
"$cli" generate \
  --input "$directory/openapi.json" \
  --target javascript \
  --output "$directory/src/generated/todo-sdk"
