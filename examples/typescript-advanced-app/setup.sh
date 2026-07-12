#!/usr/bin/env bash
set -euo pipefail

directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cli="${SDKGEN_BIN:-openapi-sdkgen}"

rm -rf "$directory/src/generated/widget-sdk"
"$cli" generate \
  --input "$directory/openapi.json" \
  --target typescript \
  --output "$directory/src/generated/widget-sdk"
pnpm --dir "$directory" install --frozen-lockfile
pnpm --dir "$directory" run build
