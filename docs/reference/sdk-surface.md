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
  Enums,
  isAPIError,
  isErrorCategory,
  isErrorCode,
  TransportErrorCode,
  type Components,
  type ClientOptions,
  type Operations,
} from "./generated/api";
```

`createClient(options)` returns the resource API plus stable namespaces:

### Exact type catalogs

`Components` preserves every component schema key and exposes its directional
projections. `Enums` exposes exact readonly runtime tuples for component enum
values. `Operations` preserves every `operationId` and correlates its `input`,
`resourceInput`, `options`, `output`, `error`, `rawResponse`, `call`,
`resourceCall`, and `pagination` slots:

```ts
type MoneyInput = Components["Money"]["input"];
type MoneyOutput = Components["Money"]["output"];
type GetPetInput = Operations["get-pet"]["input"];
type GetPetError = Operations["get-pet"]["error"];
const firstCurrency = Enums["Currency"][0];
```

Exact OpenAPI names are never normalized into flat exported type aliases.
Bracket access keeps names such as `"get-pet"` and `"get_pet"` distinct.

### Resource properties

Ergonomic resource-oriented calls derived from operation paths.

### `$operations`

Every visible OpenAPI `operationId`, exactly named and complete. For example,
`api.$operations["get-pet"](...)`. The path-derived resource tree remains an
ergonomic convenience; collisions there are resolved deterministically without
removing anything from `$operations`.

### `$links`

Typed OpenAPI Link follow-up helpers.

### `$streams`

Typed lazy `AsyncIterable` methods for streaming responses.

Errors are values with a stable code and cause. Use `isAPIError(error)` for the
base shape, `isErrorCode(error, code)` for an exact runtime code, and
`isErrorCategory(error, category)` for a generated server-error category.
Transport failures use exact values from `TransportErrorCode`.

Exactness also applies inside schemas and operation inputs. A property renamed
from `user_id` to `userId`, or a parameter moved between `query`,
`headerParams`, and another location, is a generated API breaking change; use
the quoted source key in the matching location container.

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

Webhook names, callback dimensions, component callback names, and security
scheme names use their exact OpenAPI spellings. Use bracket access whenever a
name is not a valid or unambiguous TypeScript property identifier.
