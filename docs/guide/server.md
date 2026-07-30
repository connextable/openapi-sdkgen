# Handle Webhooks and Callbacks

When the OpenAPI file defines a Webhook or Callback that your application
receives, add `--with server` to generate handler types and routers.

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --with server \
  --output ./src/generated/api
```

The generated code uses Fetch `Request` and `Response`. Connect it to Express,
Hono, Next.js, or another framework in your application.

## Webhooks

```ts
import {
  createWebhookRouter,
  type WebhookHandlers,
} from "./generated/api/server/webhooks";

const handlers: WebhookHandlers = {
  orderCreated: {
    POST: async ({ body }) => ({
      status: 202,
      body: { accepted: body.id },
    }),
  },
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

Map each OpenAPI Webhook name to an application path in `routes`. Your
application is also responsible for authenticating incoming requests. The
generated router parses and validates the request body before passing a typed
value to the handler.

## Callbacks

A Callback URL can depend on request data, so connect the generated handler to
the path used by your application.

```ts
import {
  createCallbackHandlers,
  type CallbackHandlers,
} from "./generated/api/server/callbacks";

const handlers: CallbackHandlers = {
  callbacks: {
    createOrder: {
      orderStatus: {
        "{$request.body#/callbackUrl}": {
          POST: async ({ body }) => ({ status: 204 }),
        },
      },
    },
  },
};

const callbacks = createCallbackHandlers(handlers);

const response =
  await callbacks.callbacks.createOrder.orderStatus[
    "{$request.body#/callbackUrl}"
  ].POST.fetch(request);
```

Callback keys preserve the `operationId`, Callback name, runtime expression,
and HTTP method declared in OpenAPI.
