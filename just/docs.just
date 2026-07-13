set quiet := true
set no-cd := true

# User-facing documentation commands. These intentionally do not use agent
# wrappers: they are the normal local workflow for the VitePress site.
install:
    CI=true corepack pnpm@11.11.0 --dir docs install --frozen-lockfile

lock:
    corepack pnpm@11.11.0 --dir docs clean --lockfile
    corepack pnpm@11.11.0 --dir docs install --lockfile-only

dev:
    corepack pnpm@11.11.0 --dir docs run docs:dev

build:
    corepack pnpm@11.11.0 --dir docs run docs:build

preview:
    corepack pnpm@11.11.0 --dir docs run docs:preview
