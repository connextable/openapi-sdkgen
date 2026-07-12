#!/usr/bin/env bash
set -euo pipefail

directory="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cli="${SDKGEN_BIN:-openapi-sdkgen}"

rm -rf "$directory/sdk"
"$cli" generate \
  --input "$directory/openapi.json" \
  --target typescript \
  --output "$directory/sdk" \
  --package-name @example/todo-sdk
pnpm --dir "$directory/sdk" install
pnpm --dir "$directory/sdk" run build
pnpm --dir "$directory" install --frozen-lockfile
pnpm --dir "$directory" run build
