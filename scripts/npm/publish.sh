#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

if [[ $# -ne 2 ]]; then
  echo "usage: scripts/npm/publish.sh <package-directory> <latest|next>" >&2
  exit 2
fi

package_dir="$1"
dist_tag="$2"
if [[ "$dist_tag" != "latest" && "$dist_tag" != "next" ]]; then
  echo "npm dist-tag must be latest or next: $dist_tag" >&2
  exit 2
fi
if [[ ! -f "$package_dir/package.json" ]]; then
  echo "npm package directory is missing package.json: $package_dir" >&2
  exit 2
fi
if [[ -z "${ACTIONS_ID_TOKEN_REQUEST_URL:-}" ]]; then
  echo "npm publishing requires a GitHub Actions OIDC token" >&2
  exit 2
fi

# actions/setup-node creates an npmrc entry that refers to NODE_AUTH_TOKEN.
# npm's trusted-publishing flow uses the GitHub OIDC token first, so remove
# legacy token variables to prevent any fallback to token authentication.
unset NODE_AUTH_TOKEN NPM_TOKEN

package_name="$(node -p "require(process.argv[1]).name" "$package_dir/package.json")"
package_version="$(node -p "require(process.argv[1]).version" "$package_dir/package.json")"
published_dist_tags="$(npm view "$package_name" dist-tags --json 2>/dev/null || true)"
current_dist_tag=""
if [[ -n "$published_dist_tags" && "$published_dist_tags" != "null" ]]; then
  current_dist_tag="$(node -e 'const tags = JSON.parse(process.argv[1]); process.stdout.write(tags[process.argv[2]] ?? "")' "$published_dist_tags" "$dist_tag")"
  if [[ -n "$current_dist_tag" && "$current_dist_tag" != "$package_version" ]]; then
    if ! node - "$current_dist_tag" "$package_version" <<'NODE'
const [published, candidate] = process.argv.slice(2);
const parse = (version) => {
  const match = version.match(/^(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/);
  if (!match) throw new Error(`invalid SemVer: ${version}`);
  return { numeric: match.slice(1, 4).map(Number), prerelease: match[4]?.split(".") ?? [] };
};
const compare = (left, right) => {
  const a = parse(left);
  const b = parse(right);
  for (let index = 0; index < 3; index += 1) {
    if (a.numeric[index] !== b.numeric[index]) return Math.sign(a.numeric[index] - b.numeric[index]);
  }
  if (a.prerelease.length === 0 || b.prerelease.length === 0) return a.prerelease.length === b.prerelease.length ? 0 : a.prerelease.length === 0 ? 1 : -1;
  for (let index = 0; index < Math.max(a.prerelease.length, b.prerelease.length); index += 1) {
    const x = a.prerelease[index];
    const y = b.prerelease[index];
    if (x === y) continue;
    if (x === undefined) return -1;
    if (y === undefined) return 1;
    const numericX = /^\d+$/.test(x);
    const numericY = /^\d+$/.test(y);
    if (numericX && numericY) return Math.sign(Number(x) - Number(y));
    if (numericX) return -1;
    if (numericY) return 1;
    return x < y ? -1 : 1;
  }
  return 0;
};
process.exit(compare(published, candidate) <= 0 ? 0 : 1);
NODE
    then
      echo "npm ${dist_tag} already points to newer version ${current_dist_tag}; refusing to downgrade it to ${package_version}" >&2
      exit 1
    fi
  fi
fi
published_integrity="$(npm view "${package_name}@${package_version}" dist.integrity --json 2>/dev/null || true)"
if [[ -n "$published_integrity" && "$published_integrity" != "null" ]]; then
  published_integrity="$(node -e '
const result = JSON.parse(process.argv[1]);
if (typeof result === "string") {
  process.stdout.write(result);
} else if (result !== null && result?.error?.code !== "E404") {
  throw new Error(`unexpected npm integrity response: ${JSON.stringify(result)}`);
}
' "$published_integrity")"
fi
if [[ -n "$published_integrity" ]]; then
  local_integrity="$(npm pack --dry-run --json "$package_dir" | node -e 'let input = ""; process.stdin.on("data", (chunk) => { input += chunk; }); process.stdin.on("end", () => process.stdout.write(JSON.parse(input)[0].integrity));')"
  if [[ "$local_integrity" == "$published_integrity" ]]; then
    if [[ "$current_dist_tag" != "$package_version" ]]; then
      echo "npm package already exists, but ${dist_tag} points to ${current_dist_tag:-nothing}; repair that dist-tag interactively before resuming" >&2
      exit 1
    fi
    printf 'ok npm package already published %s@%s\n' "$package_name" "$package_version"
    exit 0
  fi
  echo "npm already contains ${package_name}@${package_version} with different integrity" >&2
  exit 1
fi

npm publish "$package_dir" --access public --tag "$dist_tag" --ignore-scripts
printf 'ok npm publish %s\n' "$dist_tag"
