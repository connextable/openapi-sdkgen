# 생성 클라이언트 사용

## API 설정마다 클라이언트 하나 생성

```ts
import { createClient } from "./generated/api";

const api = createClient({
  baseURL: "https://api.example.test/v1",
  authorization: "Bearer example-token",
});
```

명시적인 `baseURL`은 OpenAPI
[Server Object](https://spec.openapis.org/oas/v3.2.0.html#server-object)보다 우선합니다. `baseURL`이 없으면 operation server,
path server, root server 순서로 서버를 선택합니다.

## Resource API와 정확한 operation

```ts
const created = await api.todos.create({
  body: { title: "Readable code" },
});

const page = await api.$operations.listTodos({
  query: { limit: 50 },
});
```

애플리케이션 코드에서는 보통 resource 호출을 사용하세요.
[Operation Object](https://spec.openapis.org/oas/v3.2.0.html#operation-object)의 정확한 `operationId`가 더
분명하거나 resource 이름으로 자연스럽게 매핑되지 않을 때는 `$operations`를 사용하면 됩니다.

## 미디어 선택과 raw response

여러 [Media Type Object](https://spec.openapis.org/oas/v3.2.0.html#media-type-object)를 받는
[Request Body Object](https://spec.openapis.org/oas/v3.2.0.html#request-body-object)에서는 어떤 representation을 보낼지 명시해야 합니다. raw helper는
status, header, Fetch `Response`를 함께 유지합니다.

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

## Link와 stream

Response [Link Object](https://spec.openapis.org/oas/v3.2.0.html#link-object)는 `$links` 아래에 typed follow-up helper로 생성됩니다. streaming operation은 `$streams`
아래에서 lazy typed `AsyncIterable`로 사용할 수 있습니다.

```ts
const created = await api.$operations.createOrder.raw({ body: order });
const receipt = await api.$links.createOrder.getReceipt(created);

for await (const event of api.$streams.watchOrders({ query: { cursor: "0" } })) {
  console.log(event);
}
```

Link에서 파생한 값은 기본값일 뿐이며, 호출할 때 전달한 명시적인 입력이 항상 우선합니다. stream decode
error는 iteration 중에 발생하고 취소는 일반 `AbortSignal` 경로를 사용합니다. 호스트 환경과의 연동은
[전송·인증·스트림](./transport.md)에서 확인하세요.
