#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

TAG_INPUT="${1:-}"
DRY_RUN=0
TAG_REGEX='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+)(\.[0-9A-Za-z-]+)*)?$'
STABLE_TAG_REGEX='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'

usage() {
  cat <<'EOF'
Usage: just release [--dry-run|-n|patch|minor|major|vX.Y.Z[-prerelease]]

Validate the current main commit, create an annotated release tag, and push
main and the tag atomically. With no argument, recommend a version from
conventional commits. The first release defaults to v0.1.0.
EOF
}

fail() {
  echo "release: $*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

latest_stable_tag() {
  git tag --merged HEAD --list 'v*' --sort=-v:refname | while IFS= read -r tag; do
    if [[ "$tag" =~ $STABLE_TAG_REGEX ]]; then
      printf '%s\n' "$tag"
      return 0
    fi
  done
}

semver_parts() {
  local tag="$1"
  local version="${tag#v}"
  version="${version%%-*}"
  IFS=. read -r major minor patch <<<"$version"
  printf '%s %s %s\n' "$major" "$minor" "$patch"
}

validate_prerelease() {
  local tag="$1"
  local prerelease
  local identifier

  [[ "$tag" == *-* ]] || return
  prerelease="${tag#*-}"
  IFS=. read -r -a identifiers <<<"$prerelease"
  for identifier in "${identifiers[@]}"; do
    if [[ "$identifier" =~ ^[0-9]+$ && "$identifier" =~ ^0[0-9]+$ ]]; then
      fail "numeric prerelease identifiers must not have leading zeroes: $tag"
    fi
  done
}

prerelease_part() {
  local tag="$1"

  if [[ "$tag" == *-* ]]; then
    printf '%s\n' "${tag#*-}"
  fi
}

semver_is_greater() {
  local candidate="$1"
  local baseline="$2"
  local candidate_major
  local candidate_minor
  local candidate_patch
  local baseline_major
  local baseline_minor
  local baseline_patch
  local candidate_prerelease
  local baseline_prerelease
  local candidate_identifier
  local baseline_identifier
  local index=0

  read -r candidate_major candidate_minor candidate_patch <<<"$(semver_parts "$candidate")"
  read -r baseline_major baseline_minor baseline_patch <<<"$(semver_parts "$baseline")"
  if ((candidate_major != baseline_major)); then
    ((candidate_major > baseline_major))
    return
  fi
  if ((candidate_minor != baseline_minor)); then
    ((candidate_minor > baseline_minor))
    return
  fi
  if ((candidate_patch != baseline_patch)); then
    ((candidate_patch > baseline_patch))
    return
  fi

  candidate_prerelease="$(prerelease_part "$candidate")"
  baseline_prerelease="$(prerelease_part "$baseline")"
  if [[ -z "$candidate_prerelease" ]]; then
    [[ -n "$baseline_prerelease" ]]
    return
  fi
  [[ -n "$baseline_prerelease" ]] || return 1

  IFS=. read -r -a candidate_identifiers <<<"$candidate_prerelease"
  IFS=. read -r -a baseline_identifiers <<<"$baseline_prerelease"
  while :; do
    candidate_identifier="${candidate_identifiers[$index]:-}"
    baseline_identifier="${baseline_identifiers[$index]:-}"
    if [[ -z "$candidate_identifier" ]]; then
      [[ -n "$baseline_identifier" ]] && return 1
      return 1
    fi
    [[ -n "$baseline_identifier" ]] || return 0
    if [[ "$candidate_identifier" == "$baseline_identifier" ]]; then
      index=$((index + 1))
      continue
    fi
    if [[ "$candidate_identifier" =~ ^[0-9]+$ && "$baseline_identifier" =~ ^[0-9]+$ ]]; then
      ((10#$candidate_identifier > 10#$baseline_identifier))
      return
    fi
    if [[ "$candidate_identifier" =~ ^[0-9]+$ ]]; then
      return 1
    fi
    if [[ "$baseline_identifier" =~ ^[0-9]+$ ]]; then
      return 0
    fi
    [[ "$candidate_identifier" > "$baseline_identifier" ]]
    return
  done
}

latest_reachable_tag() {
  local latest=""
  local tag

  while IFS= read -r tag; do
    [[ "$tag" =~ $TAG_REGEX ]] || continue
    validate_prerelease "$tag"
    if [[ -z "$latest" ]] || semver_is_greater "$tag" "$latest"; then
      latest="$tag"
    fi
  done < <(git tag --merged HEAD --list 'v*')
  printf '%s\n' "$latest"
}

next_version() {
  local base="$1"
  local bump="$2"
  local major
  local minor
  local patch

  if [[ -z "$base" ]]; then
    case "$bump" in
      patch|minor) printf '%s\n' 'v0.1.0' ;;
      major) printf '%s\n' 'v1.0.0' ;;
      *) fail "unknown version bump: $bump" ;;
    esac
    return
  fi

  read -r major minor patch <<<"$(semver_parts "$base")"
  case "$bump" in
    patch) patch=$((patch + 1)) ;;
    minor)
      minor=$((minor + 1))
      patch=0
      ;;
    major)
      major=$((major + 1))
      minor=0
      patch=0
      ;;
    *) fail "unknown version bump: $bump" ;;
  esac
  printf 'v%s.%s.%s\n' "$major" "$minor" "$patch"
}

recommended_bump() {
  local range="$1"
  local messages
  local subjects

  messages="$(git log --format='%s%n%b' "$range")"
  subjects="$(git log --format='%s' "$range")"
  if grep -Eq 'BREAKING CHANGE:|^[[:alpha:]]+(\([^)]*\))?!:' <<<"$messages"; then
    printf '%s\n' 'major'
  elif grep -Eq '^feat(\([^)]*\))?:' <<<"$subjects"; then
    printf '%s\n' 'minor'
  else
    printf '%s\n' 'patch'
  fi
}

resolve_tag() {
  local input="$1"
  local latest_stable="$2"
  local latest_release="$3"
  local range="$4"
  local bump
  local baseline

  if [[ -z "$input" ]]; then
    if [[ -z "$latest_release" ]]; then
      printf '%s\n' 'v0.1.0'
      return
    fi
    if [[ -n "$(prerelease_part "$latest_release")" ]]; then
      printf '%s\n' "${latest_release%%-*}"
      return
    fi
    bump="$(recommended_bump "$range")"
    next_version "$latest_stable" "$bump"
    return
  fi

  case "$input" in
    patch|minor|major)
      baseline="$latest_release"
      next_version "$baseline" "$input"
      ;;
    *)
      [[ "$input" =~ $TAG_REGEX ]] || fail "version must be patch, minor, major, or vX.Y.Z[-prerelease]"
      validate_prerelease "$input"
      if [[ -n "$latest_release" ]] && ! semver_is_greater "$input" "$latest_release"; then
        fail "version must be newer than reachable tag $latest_release: $input"
      fi
      printf '%s\n' "$input"
      ;;
  esac
}

confirm() {
  local tag="$1"
  local response

  if [[ ! -t 0 ]]; then
    fail "interactive confirmation requires a terminal"
  fi

  read -r "response?Create and push ${tag}? [Y/n]: "
  case "${response:-y}" in
    y|Y|yes|YES) ;;
    *)
      echo 'release: aborted; no tag created'
      exit 0
      ;;
  esac
}

require_command git
require_command just

case "$TAG_INPUT" in
  --dry-run|-n)
    DRY_RUN=1
    TAG_INPUT=''
    ;;
esac

case "$TAG_INPUT" in
  ''|patch|minor|major) ;;
  --help|-h)
    usage
    exit 0
    ;;
  *) [[ "$TAG_INPUT" =~ $TAG_REGEX ]] || fail "unknown release argument: $TAG_INPUT" ;;
esac

[[ "$(git branch --show-current)" == 'main' ]] || fail "release must run from main"
if [[ -n "$(git status --porcelain)" ]]; then
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo 'release: dry run continues with an uncommitted working tree'
  else
    fail "working tree must be clean"
  fi
fi
git remote get-url origin >/dev/null 2>&1 || fail "origin remote is required"

git fetch --tags --prune origin
git show-ref --verify --quiet refs/remotes/origin/main || fail "origin/main is missing"
git merge-base --is-ancestor origin/main HEAD || fail "local main is behind or diverged from origin/main"

latest_tag="$(latest_stable_tag || true)"
latest_release_tag="$(latest_reachable_tag || true)"
if [[ -n "$latest_tag" ]]; then
  commit_range="${latest_tag}..HEAD"
else
  commit_range='HEAD'
fi

release_tag="$(resolve_tag "$TAG_INPUT" "$latest_tag" "$latest_release_tag" "$commit_range")"
git rev-parse --verify --quiet "refs/tags/${release_tag}" >/dev/null && fail "local tag already exists: ${release_tag}"
git ls-remote --exit-code --tags origin "refs/tags/${release_tag}" >/dev/null 2>&1 && fail "remote tag already exists: ${release_tag}"

release_commit="$(git rev-parse HEAD)"
echo "Release target: ${release_commit:0:12}"
echo "Previous stable tag: ${latest_tag:-none}"
echo "Previous release tag: ${latest_release_tag:-none}"
echo "Release tag: ${release_tag}"
echo 'Commits:'
git log --format='  %h %s' "$commit_range"

if [[ "$DRY_RUN" -eq 1 ]]; then
  echo 'release: dry run; no checks, tag, or push'
  exit 0
fi

just agent check
[[ "$(git branch --show-current)" == 'main' ]] || fail "branch changed while checks ran"
[[ "$(git rev-parse HEAD)" == "$release_commit" ]] || fail "HEAD changed while checks ran"
[[ -z "$(git status --porcelain)" ]] || fail "working tree changed while checks ran"
confirm "$release_tag"

release_notes="$(printf 'Release %s\n\n' "$release_tag")"
release_notes+="$(git log --format='- %s' "$commit_range")"
git tag -a "$release_tag" -m "$release_notes" "$release_commit"
if ! git push --atomic origin "$release_commit:refs/heads/main" "refs/tags/${release_tag}:refs/tags/${release_tag}"; then
  git tag -d "$release_tag" >/dev/null 2>&1 || true
  fail "push failed; removed local tag ${release_tag}"
fi

echo "release: created and pushed ${release_tag}"
