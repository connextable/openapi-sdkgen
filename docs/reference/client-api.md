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

## TypeScript types

The generated SDK exposes component-, route-, operation-, request-section-, and
parameter-based type helpers. See
[Generated TypeScript types](./typescript-types.md) for the complete type API and
examples.

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

When an operation has several OpenAPI security alternatives, select one with
`securityRequirement`. A sole requirement is selected automatically, and an empty
requirement is named `"anonymous"`.

```ts
await api.$operations.updateCheckout({
  securityRequirement: "GuestCapability",
  authorization: "Bearer example-token",
});
```

Use `securityProvider` to load credentials for the selected requirement. See
[Authentication](../guide/transport.md#authentication) for examples.

## Request headers

Every declared header appears under `headerParams`. Headers controlled by Fetch are
optional caller inputs, and the active Fetch implementation decides whether they are
sent. See [Request headers](../guide/transport.md#request-headers).

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
