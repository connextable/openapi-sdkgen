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

Client-only output contains generated `.ts` source in `index.ts` and `generated/*.ts`; `--with server` additionally emits `server/*.ts`. Neither mode contains a `package.json`, build configuration, or dependencies: your application installs TypeScript and compiles this source as part of its ordinary build. Every generated file begins with generated-code markers and supported lint suppression directives. Prettier has no file-level in-source ignore directive, so add the generated directory (for example `src/generated/**`) to your application's `.prettierignore`.

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
import { createWebhookRouter, type WebhookHandlers } from "./generated/api/server/webhooks.js";

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
import { createCallbackHandlers } from "./generated/api/server/callbacks.js";

const callbacks = createCallbackHandlers({
  orderStatus: async ({ body }) => ({ status: 204 }),
});
await callbacks.orderStatus.fetch(request);
```

Generate separate source trees by invoking the command once per OpenAPI document and output directory.

Lossless OpenAPI metadata stays out of the normal SDK root surface. Tooling can
opt in through its explicit entry:

```ts
import { openapi } from "./generated/api/metadata.js";

openapi.document;
openapi.version;
openapi.versionLine;
```

## Generate JavaScript source

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target javascript \
  --output ./src/generated/api
```

JavaScript output is native ESM (`index.js` and `generated/*.js`) with no
build step or package metadata. It includes adjacent `.d.ts` sidecars, so a
`// @ts-check` JavaScript app gets the same typed resource calls as TypeScript.
The exact `operationId` surface remains available under `$operations`:

```js
import { createClient } from "./generated/api/index.js";

const api = createClient({ baseURL: "https://api.example.test" });
const todo = await api.todos.create({ body: { title: "Read docs" } });
const result = await api.$operations.listWidgets({ query: { limit: 20 } });
```

## Runnable TypeScript example

[`examples/typescript-todo-app`](examples/typescript-todo-app) reproduces the complete consumer flow without test fixtures or symlinks: it generates `src/generated/todo-sdk/` from a small Todo OpenAPI document, builds the app, and calls it over a local HTTP server.

[`examples/typescript-advanced-app`](examples/typescript-advanced-app) covers pagination, path/query/header/cookie inputs, raw responses, typed errors, binary bodies, authorization, and timeouts against a local HTTP server.

[`examples/javascript-todo-app`](examples/javascript-todo-app) runs a native
JavaScript app against a separate local server with no package install or
consumer build step.

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
just agent example-javascript
just agent example-capabilities
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
