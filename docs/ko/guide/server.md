# Webhook과 Callback 처리

애플리케이션이 받아야 할 Webhook이나 Callback이 OpenAPI 파일에 정의되어 있다면
`--with server`를 추가해 핸들러 타입과 라우터를 생성할 수 있습니다.

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --with server \
  --output ./src/generated/api
```

생성된 코드는 Fetch `Request`와 `Response`를 사용합니다. Express, Hono, Next.js
등 웹 프레임워크에 연결하는 코드는 애플리케이션에서 작성합니다.

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

`routes`에는 OpenAPI에 정의된 Webhook 이름과 실제 애플리케이션 경로를
연결합니다. 요청 인증도 애플리케이션에서 처리해야 합니다. 생성된 라우터는 요청
본문을 파싱하고 OpenAPI 스키마에 맞는지 검사한 뒤 타입이 지정된 값으로
핸들러에 전달합니다.

## Callback

Callback URL은 요청 데이터에 따라 달라질 수 있으므로 애플리케이션에서 사용할
경로에 생성된 핸들러를 직접 연결해야 합니다.

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

Callback API의 키는 OpenAPI에 선언된 `operationId`, Callback 이름, 런타임
표현식, HTTP 메서드를 그대로 사용합니다.
