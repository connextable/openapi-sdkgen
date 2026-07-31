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
  type LinkCalls,
  type Operations,
  type PaginateCall,
  type RawCall,
  type ResourceCall,
  type RouteContract,
  type Routes,
  type StreamCall,
} from "./generated/api";
```

- `Components`: component schemas and their input and output types
- `Enums`: runtime values for component enums
- `Routes`: types for every API, keyed by HTTP method and OpenAPI path
- `Operations`: types for APIs that declare an `operationId`
- `RouteContract<Route>`: the complete contract for one exact route
- `ResourceCall<Route>`: the callable type used by a resource method
- `RawCall<Route>`, `PaginateCall<Route>`, `LinkCalls<Route>`, and
  `StreamCall<Route>`: route-keyed capability call types

```ts
type MoneyInput = Components["Money"]["input"];
type MoneyOutput = Components["Money"]["output"];
type ListPetsInput = Routes["GET /pets"]["input"];
type GetPetInput = Operations["get-pet"]["input"];
type CreateOrder = RouteContract<"POST /orders">;
type CreateOrderInput = CreateOrder["input"];
type CreateOrderOutput = CreateOrder["output"];
type CreateOrderCall = ResourceCall<"POST /orders">;
type CreateOrderRawCall = RawCall<"POST /orders">;
const firstCurrency = Enums["Currency"][0];
```

Prefer `RouteContract<Route>` and its named slots when extracting several
types for one route. This keeps input, output, options, errors, raw responses,
and optional capabilities under one route identity instead of adding separate
helpers such as `RouteInput<Route>` and `RouteOutput<Route>`. Capability slots
and their matching helper types resolve to `never` when the route does not
declare that capability.

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

## Security requirements

When an operation declares OpenAPI Security Requirement Objects, its generated
request options expose `securityRequirement` only when more than one effective
requirement exists. The property is a required exact union of that operation's
stable requirement IDs, and the operation options argument is required. An
empty `{}` requirement is represented by `"anonymous"`. The SDK automatically
selects exactly one effective requirement. Operations with zero or one do not
expose a selector.

```ts
await api.$operations.updateCheckout({
  securityRequirement: "GuestCapability",
  authorization: "Bearer example-token",
});
```

`ClientOptions.credentials` and per-request `credentials` are Fetch
`RequestCredentials`. Use `ClientOptions.securityProvider` for host-owned
credential acquisition after request selection. It receives one selected
`requirement` and returns a scheme-keyed credential map. It cannot select a
requirement. An omitted ambiguous selection fails before the provider and
Fetch. See
[Select a security requirement and provide credentials](../guide/transport.md#select-a-security-requirement-and-provide-credentials).

## Request headers

Every declared header appears under `headerParams`. Headers controlled by Fetch,
such as `Origin`, `Host`, `Cookie`, and `Sec-*`, are optional caller inputs even
when OpenAPI marks them as required. Explicit typed values, undeclared raw
headers, and header API-key credentials are forwarded to the active Fetch
implementation. Declared/reserved raw-header collisions remain errors.

Method-override headers retain their OpenAPI requiredness and reach Fetch
without SDK-side value filtering. Links read request-header sources from
`invocation.sourceInput`, which must be passed explicitly when following the
Link; source calls do not retain their input automatically. Target values are
forwarded through the same path.

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

Security selection uses `SECURITY_REQUIREMENT_REQUIRED` and
`SECURITY_REQUIREMENT_INVALID`. Credential acquisition and application use
`SECURITY_CREDENTIALS_REQUIRED` and `SECURITY_CREDENTIALS_INVALID`.

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
