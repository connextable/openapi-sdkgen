# Inbound server contracts

Generate this add-on only when the OpenAPI document contains
[Callback Objects](https://spec.openapis.org/oas/v3.2.0.html#callback-object)
or root `webhooks` in the
[OpenAPI Object](https://spec.openapis.org/oas/v3.2.0.html#openapi-object) that
the application will receive:

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --with server \
  --output ./src/generated/api
```

The add-on uses Fetch `Request` and `Response` only. Framework-specific routing
stays in application code.

## Webhooks

```ts
import {
  createWebhookRouter,
  type WebhookHandlers,
} from "./generated/api/server/webhooks";

const handlers: WebhookHandlers = {
  orderCreated: async ({ body }) => ({
    status: 202,
    body: { accepted: body.id },
  }),
};

const router = createWebhookRouter(handlers, {
  routes: { orderCreated: "/webhooks/orders" },
  authenticate: ({ request }) =>
    request.headers.get("x-signature") === expectedSignature
      ? undefined
      : new Response("Unauthorized", { status: 401 }),
});

const response = await router.fetch(request);
```

The application maps webhook identifiers to concrete routes and authenticates
the request. Generated code parses, validates, and types the declared payload.

## Callbacks

Callback URLs are runtime expressions, so the host mounts an explicit generated
Fetch handler at its chosen route:

```ts
import { createCallbackHandlers } from "./generated/api/server/callbacks";

const callbacks = createCallbackHandlers({
  orderStatus: async ({ body }) => ({ status: 204 }),
});

const response = await callbacks.orderStatus.fetch(request);
```

This separation keeps the default SDK safe to import in browser bundles while
making inbound endpoints straightforward to adapt to any Fetch-capable server.
