# AGENTS.md

## Scope

This repository uses agent-safe command wrappers.

## Command rules

- Use `just agent ...` for project operations.
- Do not use `rtk` for project operations after bootstrap.
- Do not run direct `go test`, `go build`, `go run`, `pnpm`, or `npm` commands when an agent wrapper exists.
- Read-only inspection commands such as `sed`, `rg`, `ls`, and `git status` are allowed.
- Do not commit unless the user explicitly asks.

## Agent commands

```txt
just agent check
just agent fmt
just agent fmt-check
just agent vet
just agent test
just agent build
just agent mod-tidy
just agent mod-tidy-check
just agent mod-verify
just agent generate INPUT OUTPUT [TARGET]
just agent conformance
just agent ts-lock
just agent ts-install
just agent ts-fmt
just agent ts-fmt-check
just agent ts-lint
just agent ts-typecheck
just agent ts-test
just agent example-todo
just agent example-advanced
just agent example-capabilities
just agent release-check
just agent release-script-test
just agent npm-package VERSION
just agent npm-package-check [PACKAGE_DIRECTORY]
just agent npm-publish PACKAGE_DIRECTORY [latest|next]
```

## Documentation commands

VitePress uses normal user-facing commands, not `scripts/agent` wrappers:

```txt
just docs install
just docs dev
just docs build
just docs preview
just docs lock
```

## User release command

```txt
just release [patch|minor|major|vX.Y.Z[-prerelease]]
just release -- [--dry-run|-n] [--yes|-y] [--since TAG] [--resume TAG] [patch|minor|major|vX.Y.Z[-prerelease]]
```

`just release` is the user-facing release command, not an agent wrapper. It
shows the commits and release-note base, recommends a conventional-commit bump,
runs the full agent check, then atomically pushes `main` and the annotated tag.

## Output policy

- Agent scripts print short success summaries.
- On failure, scripts print the failing command and a bounded tail of a log under `.tmp/agent-logs/`.
- Use `VERBOSE=1` only for detailed output.
