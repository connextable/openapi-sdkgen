# openapi-sdkgen

`openapi-sdkgen` compiles an OpenAPI 3.2 document into typed TypeScript source for your application.

The primary distribution is a precompiled CLI binary published through GitHub Releases, so TypeScript consumers do not need Go installed. Go users can also install the command from the module:

```sh
go install github.com/connextable/openapi-sdkgen/cmd/openapi-sdkgen@latest
```

## Generate TypeScript source

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api
```

Import it from application source with a relative ESM path:

```ts
import { createClient } from "./generated/api/index.js";
```

The output directory must be fresh. The CLI stages every artifact and publishes it only after generation succeeds, rather than modifying an existing package tree.

The output contains only generated `.ts` source (`index.ts` and `generated/*.ts`). It has no `package.json`, build configuration, or dependencies: your application installs TypeScript and compiles this source as part of its ordinary build. Every generated file begins with generated-code markers and supported lint suppression directives. Prettier has no file-level in-source ignore directive, so add the generated directory (for example `src/generated/**`) to your application's `.prettierignore`.

Generate separate source trees by invoking the command once per OpenAPI document and output directory.

## Runnable TypeScript example

[`examples/typescript-todo-app`](examples/typescript-todo-app) reproduces the complete consumer flow without test fixtures or symlinks: it generates `src/generated/todo-sdk/` from a small Todo OpenAPI document, builds the app, and calls it over a local HTTP server.

[`examples/typescript-advanced-app`](examples/typescript-advanced-app) covers pagination, path/query/header/cookie inputs, raw responses, typed errors, binary bodies, authorization, and timeouts against a local HTTP server.

For repository validation, run:

```sh
just agent example-todo
just agent example-advanced
```

For normal use, follow the example's [`setup.sh`](examples/typescript-todo-app/setup.sh) and separate server/client commands.

## Architecture

```txt
OpenAPI 3.2 document
        │
        ▼
parser + validation → language-neutral IR → built-in target registry
                                               └─ typescript
```

Targets are compiled into the binary. Adding a future Kotlin, Swift, or Go target implements the target interface; it does not change OpenAPI parsing or CLI command flow.

## Development

All project operations use the agent-safe `just agent` commands.

```sh
just agent ts-lock      # update test-only TypeScript lockfile intentionally
just agent ts-install
just agent check
```

`just agent conformance` builds the CLI, generates a generic source fixture into an ignored directory, typechecks consumer tests, and runs its runtime tests. The fixture is imported with the same relative path pattern used by an application.
