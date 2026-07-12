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
just agent example-javascript
just agent example-capabilities
just agent release-check
```

## Output policy

- Agent scripts print short success summaries.
- On failure, scripts print the failing command and a bounded tail of a log under `.tmp/agent-logs/`.
- Use `VERBOSE=1` only for detailed output.
