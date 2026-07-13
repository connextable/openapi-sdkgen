# TypeScript OpenAPI Capability Showcase

This is a full source-mode application, not a test fixture. It generates a
TypeScript client and the optional inbound server contracts from
[`openapi.json`](openapi.json), compiles both as ordinary application source,
then exercises them against local HTTP servers.

It intentionally uses OpenAPI 3.2 because that is the newest supported line:
the document includes the 3.2-only `query` operation and an
`additionalOperations` entry. The CLI also accepts 3.0.x and 3.1.x documents;
the version-specific parsing and output checks live in the repository
conformance suite.

## Covered generated behavior

- Resource-oriented calls and the exact-operation `$operations` surface.
- Every generated HTTP method: `GET`, `PUT`, `POST`, `DELETE`, `OPTIONS`,
  `HEAD`, `PATCH`, `TRACE`, `QUERY`, and a custom `PURGE` operation.
- Path, query, header, cookie, and JSON-content parameters, including label,
  matrix, form, deep-object, space-delimited, and pipe-delimited styles.
- JSON, text, binary, URL-encoded form, and multipart request/response codecs.
- Component schemas, parameters, request bodies, responses, callback, and path
  item references; output/input projections; enums; typed errors; and raw
  response metadata.
- Cursor pagination, explicit `openapi` metadata, root Webhook routing with
  host-owned authentication, and host-bound Fetch Callback endpoints.

This does not claim that every valid OpenAPI grammar construct has a runtime
implementation. Unsupported constructs fail generation with a feature-path
diagnostic; see the repository [capability matrix](../../docs/openapi-feature-matrix.md).

## Run

Install `openapi-sdkgen`, Node 22+, and pnpm 10+. The CLI is resolved from
`PATH`, exactly as it would be in a normal application repository.

```sh
./setup.sh

# terminal 1
pnpm run server

# terminal 2
CAPABILITIES_API_BASE_URL=http://127.0.0.1:18790/v1 \
CAPABILITIES_WEBHOOK_BASE_URL=http://127.0.0.1:18791 \
pnpm run client
```

`src/client.ts` is deliberately written as readable, checked application code.
This example runs as compiled Node ESM, so it uses explicit `.js` file paths.
In a normal web bundler, import `./generated/capabilities-sdk` and
`./generated/capabilities-sdk/metadata` without those suffixes instead:

```ts
import { createClient } from "./generated/capabilities-sdk/index.js";
import { openapi } from "./generated/capabilities-sdk/metadata.js";
```

`src/server.ts` imports each explicit inbound contract:

```ts
import { createWebhookRouter } from "./generated/capabilities-sdk/server/webhooks.js";
import { createCallbackHandlers } from "./generated/capabilities-sdk/server/callbacks.js";
```

Webhook names are OpenAPI identifiers, so the host maps them to paths and owns
signature verification. Callback URL expressions are runtime values, so the host
mounts the matching generated Fetch endpoint at its own concrete route. The
example creates an item, delivers its Callback to that endpoint, then sends a
separate Webhook delivery.
