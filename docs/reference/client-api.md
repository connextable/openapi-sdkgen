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

Every declared header appears under `headerParams`. Headers controlled by Fetch,
such as `Origin`, `Host`, `Cookie`, and `Sec-*`, are optional caller inputs even
when OpenAPI marks them as required. Explicit typed values, undeclared raw
headers, and header API-key credentials are forwarded to the active Fetch
implementation. Declared/reserved raw-header collisions remain errors.

Method-override headers retain their OpenAPI requiredness and reach Fetch
without SDK-side value filtering. Links read request-header sources from the
original invocation input and forward target values through the same path.

The original requiredness remains available through `openapi.document`.
Generated Webhook and Callback server-add-on input types and runtime validation
preserve the full inbound contract.

See [Caller inputs and environment-controlled headers](../guide/transport.md#caller-inputs-and-environment-controlled-headers)
for final Fetch behavior and custom transport boundaries.

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
