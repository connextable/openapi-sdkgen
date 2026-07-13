# openapi-sdkgen

`openapi-sdkgen` compiles OpenAPI 3.0.x, 3.1.x, and 3.2.x documents into
source-mode TypeScript SDKs for your application.

The primary distribution is a precompiled CLI binary published through GitHub Releases, so TypeScript consumers do not need Go installed. Go users can also install the command from the module:

```sh
go install github.com/connextable/openapi-sdkgen/cmd/openapi-sdkgen@latest
```

For Node-based projects, run the same precompiled CLI directly from npm:

```sh
pnpm dlx openapi-sdkgen generate \
  --input ./openapi.yaml \
  --target typescript \
  --output ./src/generated/api
```

On macOS or Linux with Homebrew, install the same CLI with:

```sh
brew install connextable/tap/openapi-sdkgen
```

## Generate TypeScript source

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api
```

Import it from application source with the relative path your web bundler
resolves:

```ts
import { createClient } from "./generated/api";
```

Vite, Next.js, Nuxt, and similar bundlers resolve the generated directory to
its `index.ts` entry. If an application compiles and executes directly in Node
ESM instead, use the explicit `./generated/api/index.js` path.

The output directory must be fresh. The CLI stages every artifact and publishes it only after generation succeeds, rather than modifying an existing package tree.

`--input` accepts a local path, `file://` URL, HTTP(S) URL, or `-` for stdin.
An HTTP(S) input is the root document, not a remote `$ref`, so local OpenAPI
servers work without a remote-reference allowlist:

```sh
openapi-sdkgen generate \
  --input http://localhost:4010/openapi.json \
  --target typescript \
  --output ./src/generated/api
```

Pipe any other document source through stdin. When it contains relative `$ref`
values, pass `--input-base <path-or-url>` as the original document location.

### Locked remote references and schema vocabularies

Compilation is offline by default. To resolve a remote `$ref`, explicitly allow
its exact HTTPS origin and create the integrity lock once:

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api \
  --allow-remote-ref https://schemas.example.test \
  --update-ref-lock
```

This writes `openapi.json.openapi-sdkgen.lock` only because update mode was
explicitly selected. Later generation verifies every remote response digest;
use `--offline` to serve those locked resources from the adjacent
`.openapi-sdkgen-cache/` without a network request. Remote URLs require HTTPS,
an exact allowlisted origin, public DNS addresses, bounded redirects, and no
credentials.

A required custom JSON Schema vocabulary must have a checked-in trusted local
extension manifest:

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api \
  --schema-extension ./schema-extension.json \
  --update-ref-lock
```

The manifest declares vocabulary URIs, an executable plus argument array, and
its SHA-256 digest. The compiler runs versioned JSON-RPC `describe` and
`lower` calls only during generation. `lower` receives the vocabulary URI,
schema JSON, target (`typescript`), and JSON Pointer location, then returns a
replacement JSON Schema object or boolean. Generated SDK code never executes
an extension.

Client-only output contains generated `.ts` source in `index.ts` and `generated/*.ts`; `--with server` additionally emits `server/*.ts`. Neither mode contains a `package.json`, build configuration, or dependencies: your application installs TypeScript and compiles this source as part of its ordinary build. Every generated file begins with generated-code markers and supported lint suppression directives. Files also begin with Prettier's `@noprettier` pragma. To honor it, enable `checkIgnorePragma` in Prettier 3.6.0 or later; older Prettier versions can ignore the generated directory (for example `src/generated/**`) in `.prettierignore`.

### Optional inbound server contracts

Keep the default client-only entry point, or add Fetch-native Webhook and
host-bound Callback contracts explicitly:

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --with server \
  --output ./src/generated/api
```

This adds explicit `server/webhooks.ts` and `server/callbacks.ts`. `index.ts`
remains client-only, so browser imports do not pull in inbound code. Import the
role-specific entry and let the host own credential verification and routing:

```ts
import { createWebhookRouter, type WebhookHandlers } from "./generated/api/server/webhooks";

const handlers: WebhookHandlers = {
  orderCreated: async ({ body }) => ({ status: 202, body: { accepted: body.id } }),
};
const router = createWebhookRouter(handlers, {
  routes: { orderCreated: "/webhooks/orders" },
  authenticate: ({ request }) =>
    request.headers.get("x-signature") === expectedSignature
      ? undefined
      : new Response("Unauthorized", { status: 401 }),
});
await router.fetch(request);
```

Callbacks have runtime URL expressions, so the host chooses the route and mounts
a generated Fetch endpoint instead of receiving a fabricated route matcher:

```ts
import { createCallbackHandlers } from "./generated/api/server/callbacks";

const callbacks = createCallbackHandlers({
  orderStatus: async ({ body }) => ({ status: 204 }),
});
await callbacks.orderStatus.fetch(request);
```

Generate separate source trees by invoking the command once per OpenAPI document and output directory.

Lossless OpenAPI metadata stays out of the normal SDK root surface. Tooling can
opt in through its explicit entry:

```ts
import { openapi } from "./generated/api/metadata";

openapi.document;
openapi.version;
openapi.versionLine;
```

## Runnable TypeScript example

[`examples/typescript-todo-app`](examples/typescript-todo-app) reproduces the complete consumer flow without test fixtures or symlinks: it generates `src/generated/todo-sdk/` from a small Todo OpenAPI document, builds the app, and calls it over a local HTTP server.

[`examples/typescript-advanced-app`](examples/typescript-advanced-app) covers pagination, path/query/header/cookie inputs, raw responses, typed errors, binary bodies, authorization, and timeouts against a local HTTP server.

[`examples/typescript-openapi-capabilities-app`](examples/typescript-openapi-capabilities-app)
is the comprehensive TypeScript showcase. It generates client and `--with server`
source, then runs parameter styles, every generated HTTP method, all supported
body codecs, components, typed errors, pagination, explicit metadata, Callback
endpoints, and a Webhook router against local servers. It documents the unsupported
OpenAPI constructs separately instead of claiming full grammar support.

For repository validation, run:

```sh
just agent example-todo
just agent example-advanced
just agent example-capabilities
```

For normal use, follow the example's [`setup.sh`](examples/typescript-todo-app/setup.sh) and separate server/client commands.

## Architecture

```txt
OpenAPI 3.0.x / 3.1.x / 3.2.x document
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

## Documentation site

The VitePress site lives in [`docs/`](docs). Its normal local workflow is
separate from the agent command wrappers:

```sh
just docs install
just docs dev
```

Use `just docs build` for the production static build. The generated site is
written to `docs/.vitepress/dist/` and is ignored by Git.

Maintainers can refresh the pinned VitePress dependency graph with `just docs
lock`; it deliberately rebuilds the lock from the repository's supply-chain
policy before the next install.

## npm publishing

Every `v<semver>` tag builds the GitHub Release first, then packages the six
supported CLI binaries into the public `openapi-sdkgen` npm package. Stable
versions publish with the `latest` dist-tag; prereleases publish with `next`.

The npm publish job uses GitHub Actions OIDC and accepts no npm token. Before
the first automated release, bootstrap the npm package once with an interactive
2FA-protected publish, then configure its trusted publisher as follows:

- GitHub repository: `connextable/openapi-sdkgen`
- Workflow filename: `release.yml`
- Allowed action: `npm publish`

The package must already exist before npm can attach a trusted publisher. After
that one-time setup, remove any unused npm publish tokens.

## Releasing

Use the release helper instead of creating and pushing tags manually:

```sh
just release
```

It requires a clean `main` worktree, fetches current tags, recommends the next
version from conventional commits, runs `just agent check`, creates an annotated
tag, then atomically pushes `main` and the tag. The first release defaults to
`v0.1.0`.

Override the recommendation when needed:

```sh
just release patch
just release minor
just release major
just release v0.2.0-rc.1
```

The existing tag workflow handles the GitHub Release, npm publish, and Homebrew
formula update after the push succeeds.

The Homebrew formula workflow is pinned to a reviewed `homebrew-tap` commit.
Update that pin intentionally when changing the reusable formula pipeline.
