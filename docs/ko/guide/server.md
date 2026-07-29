# 인바운드 서버 계약

OpenAPI 문서에 애플리케이션이 수신할
[Callback Object](https://spec.openapis.org/oas/v3.2.0.html#callback-object) 또는
[OpenAPI Object](https://spec.openapis.org/oas/v3.2.0.html#openapi-object)의 root `webhooks`가 있을 때만 이 add-on을 생성하세요.

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --with server \
  --output ./src/generated/api
```

add-on은 Fetch `Request`와 `Response`만 사용합니다. framework별 routing은 애플리케이션 코드에서
계속 담당합니다.

## Webhook

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

애플리케이션은 webhook identifier를 concrete route로 매핑하고 요청 인증을 담당합니다. 생성 코드는
선언된 payload의 파싱, 검증, 타입 제공을 담당합니다.

## Callback

Callback URL은 runtime expression이므로 호스트가 선택한 경로에 생성된 Fetch handler를 명시적으로 mount해야 합니다.

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

생성된 callback tree는 source operation ID, Callback Object 이름, runtime expression,
HTTP method를 정확히 보존합니다. 재사용 가능한 `components.callbacks`는
`ComponentCallbacks` 타입과 `componentCallbacks` runtime catalog에서 제공합니다.

이 분리는 기본 SDK를 browser bundle에서 안전하게 import할 수 있게 하면서도, Fetch를 지원하는 어떤
server에도 인바운드 endpoint를 쉽게 연결할 수 있게 합니다.
