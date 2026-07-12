# openapi-sdkgen

`openapi-sdkgen` compiles OpenAPI 3.0.x, 3.1.x, and 3.2.x documents into
source-mode TypeScript or native JavaScript SDKs for your application.

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

Both targets also export `openapiDocument`, `openapiVersion`, and
`openapiVersionLine`. This lossless metadata surface keeps documentation,
examples, tags, and `x-*` extensions available without pretending they affect
runtime requests.

## Generate JavaScript source

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target javascript \
  --output ./src/generated/api
```

JavaScript output is native ESM (`index.js` and `generated/*.js`) with no
build step or package metadata. Import it relatively and call operations by
their exact OpenAPI `operationId`:

```js
import { createClient } from "./generated/api/index.js";

const api = createClient({ baseURL: "https://api.example.test" });
const result = await api.$operations.listWidgets({ query: { limit: 20 } });
```

## Runnable TypeScript example

[`examples/typescript-todo-app`](examples/typescript-todo-app) reproduces the complete consumer flow without test fixtures or symlinks: it generates `src/generated/todo-sdk/` from a small Todo OpenAPI document, builds the app, and calls it over a local HTTP server.

[`examples/typescript-advanced-app`](examples/typescript-advanced-app) covers pagination, path/query/header/cookie inputs, raw responses, typed errors, binary bodies, authorization, and timeouts against a local HTTP server.

[`examples/javascript-todo-app`](examples/javascript-todo-app) runs a native
JavaScript app against a separate local server with no package install or
consumer build step.

For repository validation, run:

```sh
just agent example-todo
just agent example-advanced
just agent example-javascript
```

For normal use, follow the example's [`setup.sh`](examples/typescript-todo-app/setup.sh) and separate server/client commands.

## Architecture

```txt
OpenAPI 3.0.x / 3.1.x / 3.2.x document
        │
        ▼
parser + validation → language-neutral IR → built-in target registry
                                               ├─ typescript
                                               └─ javascript
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
