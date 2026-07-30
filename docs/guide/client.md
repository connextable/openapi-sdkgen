# Use the generated client

## Configure a client

```ts
import { createClient } from "./generated/api";

const api = createClient({
  baseURL: "https://api.example.test/v1",
  authorization: "Bearer example-token",
});
```

When `baseURL` is omitted, the client uses
[Server Objects](https://spec.openapis.org/oas/v3.2.0.html#server-object) from
the OpenAPI file. A server declared for a path or operation takes precedence
over the root server.

Create a separate client for each base URL or set of credentials.

## Call an API

Resource methods are the most readable choice for normal application code.

```ts
const created = await api.todos.create({
  body: { title: "Write documentation" },
});
```

Use `$routes` to call an API by its exact HTTP method and OpenAPI path.

```ts
const todos = await api.$routes["GET /todos"]({
  query: { limit: 50 },
});
```

When the OpenAPI file declares an `operationId`, the same API is available
through `$operations`.

```ts
const todos = await api.$operations.listTodos({
  query: { limit: 50 },
});
```

## Choose a request media type

When a request body accepts multiple media types, select the one to send.

```ts
const result = await api.$operations.uploadAsset.raw({
  body: {
    contentType: "application/octet-stream",
    value: file,
  },
});
```

The `.raw()` call returns the decoded data together with the status, headers,
and Fetch `Response`.

```ts
if (result.status === 201) {
  console.log(result.headers.location);
}
```

## Links and streams

When a response defines an OpenAPI
[Link Object](https://spec.openapis.org/oas/v3.2.0.html#link-object), use
`$links` to make the follow-up request.

```ts
const created = await api.$operations.createOrder.raw({ body: order });
const receipt = await api.$links.createOrder.getReceipt(created);
```

Streaming APIs are available as `AsyncIterable` values under `$streams`.

```ts
for await (const event of api.$streams.watchOrders({
  query: { cursor: "0" },
})) {
  console.log(event);
}
```

See [transport, authentication, and streams](./transport.md) for request
cancellation and custom transport configuration.
