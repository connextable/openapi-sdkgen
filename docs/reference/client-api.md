# Generated client API

The TypeScript SDK provides separate import paths for different tasks. Most
applications only need `./generated/api`.

| Import path | Use it for |
| --- | --- |
| `./generated/api` | API calls, generated types, errors, Links, and streams |
| `./generated/api/metadata` | Reading the source OpenAPI file and version |
| `./generated/api/server/webhooks` | Handling Webhooks |
| `./generated/api/server/callbacks` | Handling Callbacks |

::: details Running directly in Node ESM

Use an explicit `.js` path when running compiled files directly in Node.

```ts
import { createClient } from "./generated/api/index.js";
```

Node ESM does not resolve a directory import to `index.js`.
:::

## Client

```ts
import { createClient } from "./generated/api";

const api = createClient({
  baseURL: "https://api.example.test/v1",
});
```

See [transport, authentication, and streams](../guide/transport.md) for client
configuration.

## Generated types

```ts
import {
  Enums,
  type Components,
  type Operations,
  type Routes,
} from "./generated/api";
```

- `Components`: component schemas and their input and output types
- `Enums`: runtime values for component enums
- `Routes`: types for every API, keyed by HTTP method and OpenAPI path
- `Operations`: types for APIs that declare an `operationId`

```ts
type MoneyInput = Components["Money"]["input"];
type MoneyOutput = Components["Money"]["output"];
type ListPetsInput = Routes["GET /pets"]["input"];
type GetPetInput = Operations["get-pet"]["input"];
const firstCurrency = Enums["Currency"][0];
```

Generated names preserve the spelling and case from OpenAPI. Use bracket
notation when a name contains characters that are not valid in a TypeScript
property name.

## Call an API

### Resource methods

Use path-based resource methods for normal application code.

```ts
const todo = await api.todos.create({
  body: { title: "Write documentation" },
});
```

### `$routes`

Call an API by its HTTP method and OpenAPI path. This also works when no
`operationId` is declared.

```ts
const todos = await api.$routes["GET /todos"]({
  query: { limit: 20 },
});
```

### `$operations`

Call an API by its declared `operationId`.

```ts
const todos = await api.$operations["listTodos"]({
  query: { limit: 20 },
});
```

## Request headers

Declared caller-controlled headers appear under `headerParams`. Fetch-managed
headers such as `Origin`, `Host`, `Cookie`, and `Sec-*` do not appear in
resource, `$routes`, or `$operations` input types, even when OpenAPI marks them
as required. Raw client and request `headers` options cannot add them either.

The OpenAPI declaration remains available through `openapi.document`.
Server-add-on input types preserve the full inbound header contract and its
requiredness. A Link that tries to read or assign a Fetch-managed request
header produces a generation diagnostic at that Link field.

See [Caller-owned and host-managed headers](../guide/transport.md#caller-owned-and-host-managed-headers)
for browser behavior, migration guidance, conditional method-override headers,
and trusted non-browser transport injection.

## Links and streams

- `$links`: follow-up requests defined by OpenAPI Links
- `$streams`: `AsyncIterable` values for streaming responses

See [Use the generated client](../guide/client.md) for examples.

## Errors

```ts
import {
  isAPIError,
  isErrorCategory,
  isErrorCode,
  TransportErrorCode,
} from "./generated/api";
```

- `isAPIError(error)`: checks for any generated API error
- `isErrorCode(error, code)`: checks an exact error code
- `isErrorCategory(error, category)`: checks an error category
- `TransportErrorCode`: lists errors raised while sending or receiving a request

## OpenAPI metadata

```ts
import { openapi } from "./generated/api/metadata";

openapi.document;
openapi.version;
openapi.versionLine;
```

`openapi.document` contains the OpenAPI content used to generate the SDK.

## Webhooks and Callbacks

When generation includes `--with server`, use these imports:

```ts
import { createWebhookRouter } from "./generated/api/server/webhooks";
import { createCallbackHandlers } from "./generated/api/server/callbacks";
```

See [Handle Webhooks and Callbacks](../guide/server.md) for examples.
