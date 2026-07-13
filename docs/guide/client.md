# Use the generated client

## Create one client per API configuration

```ts
import { createClient } from "./generated/api";

const api = createClient({
  baseURL: "https://api.example.test/v1",
  authorization: "Bearer example-token",
});
```

An explicit `baseURL` wins over OpenAPI
[Server Objects](https://spec.openapis.org/oas/v3.2.0.html#server-object). If it is omitted, the
runtime selects the operation server, then path server, then root server.

## Resource API and exact operations

```ts
const created = await api.todos.create({
  body: { title: "Readable code" },
});

const page = await api.$operations.listTodos({
  query: { limit: 50 },
});
```

Use resource calls for application code and `$operations` when an exact
`operationId` from an
[Operation Object](https://spec.openapis.org/oas/v3.2.0.html#operation-object)
is clearer or does not map cleanly to a resource name.

## Choose media and inspect raw responses

When a [Request Body Object](https://spec.openapis.org/oas/v3.2.0.html#request-body-object)
accepts several [Media Type Objects](https://spec.openapis.org/oas/v3.2.0.html#media-type-object), make the representation
explicit. The raw helper retains status, headers, and the Fetch `Response`.

```ts
const result = await api.$operations.uploadAsset.raw({
  body: {
    contentType: "application/octet-stream",
    value: file,
  },
});

if (result.status === 201) {
  console.log(result.headers.location);
}
```

## Links and streams

Response [Link Objects](https://spec.openapis.org/oas/v3.2.0.html#link-object)
become typed follow-up helpers under `$links`. Streaming
operations expose typed lazy `AsyncIterable` helpers under `$streams`.

```ts
const created = await api.$operations.createOrder.raw({ body: order });
const receipt = await api.$links.createOrder.getReceipt(created);

for await (const event of api.$streams.watchOrders({ query: { cursor: "0" } })) {
  console.log(event);
}
```

Link-derived values are defaults; explicit input overrides them. Stream decode
failures occur during iteration and cancellation uses the normal `AbortSignal`
path. See [transport, auth, and streams](./transport.md) for host integration.
