# Generated SDK surface

Each generated SDK has role-specific entry points. Import only the entry that
matches the code being written.

## Web application entry: `./generated/api`

Exports the client factory, resources, exact operations, errors, stream helpers,
and Link helpers.

## Metadata entry: `./generated/api/metadata`

Exports explicit lossless OpenAPI metadata for tooling and documentation.

## Optional inbound entries

`./generated/api/server/webhooks` exports the `--with server` Webhook router
and handler types. `./generated/api/server/callbacks` exports the matching
Callback endpoint factories and handler types.

::: details Direct Node ESM

Use the explicit `.js` file paths when compiling and running directly in Node:
`./generated/api/index.js`, `./generated/api/metadata.js`, and the matching
server entry files. Relative directory imports are a web-bundler convenience,
not a Node ESM feature.
:::

## Client entry

```ts
import {
  createClient,
  isTransportError,
  isValidationFailedError,
  type ClientOptions,
} from "./generated/api";
```

`createClient(options)` returns the resource API plus stable namespaces:

### Resource properties

Ergonomic resource-oriented calls derived from operation paths.

### `$operations`

Every visible OpenAPI `operationId`, exactly named.

### `$links`

Typed OpenAPI Link follow-up helpers.

### `$streams`

Typed lazy `AsyncIterable` methods for streaming responses.

Errors are values with a stable code and cause. `isTransportError` identifies
transport/HTTP failures; `isValidationFailedError` identifies generated schema
validation failures before a request or after a response decode.

## Metadata entry

```ts
import { openapi } from "./generated/api/metadata";

openapi.document;
openapi.version;
openapi.versionLine;
```

Metadata is deliberately outside the root client entry so ordinary application
imports do not take a dependency on the entire source document.

## Inbound entries

```ts
import { createWebhookRouter } from "./generated/api/server/webhooks";
import { createCallbackHandlers } from "./generated/api/server/callbacks";
```

These entry points exist only when generation includes `--with server`. See
[Inbound server contracts](../guide/server.md) for integration examples.
