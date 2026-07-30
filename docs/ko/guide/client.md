# 생성된 클라이언트 사용

## 클라이언트 설정

```ts
import { createClient } from "./generated/api";

const api = createClient({
  baseURL: "https://api.example.test/v1",
  authorization: "Bearer example-token",
});
```

`baseURL`을 지정하지 않으면 OpenAPI 파일의
[Server Object](https://spec.openapis.org/oas/v3.2.0.html#server-object)를
사용합니다. operation이나 경로에 별도 서버가 선언되어 있다면 더 구체적인 설정이
우선합니다.

같은 API를 여러 주소나 인증 정보로 호출해야 한다면 설정마다 클라이언트를 하나씩
만드세요.

## API 호출

일반적인 애플리케이션 코드에서는 리소스 메서드가 가장 읽기 쉽습니다.

```ts
const created = await api.todos.create({
  body: { title: "문서 작성" },
});
```

정확한 HTTP 메서드와 OpenAPI 경로로 호출하려면 `$routes`를 사용합니다.

```ts
const todos = await api.$routes["GET /todos"]({
  query: { limit: 50 },
});
```

OpenAPI 파일에 `operationId`가 있다면 `$operations`에서도 같은 API를 호출할 수
있습니다.

```ts
const todos = await api.$operations.listTodos({
  query: { limit: 50 },
});
```

## 요청 본문의 미디어 타입 선택

하나의 요청 본문이 여러 미디어 타입을 받는다면 보낼 형식을 명시합니다.

```ts
const result = await api.$operations.uploadAsset.raw({
  body: {
    contentType: "application/octet-stream",
    value: file,
  },
});
```

`.raw()`는 변환된 응답 데이터와 함께 상태 코드, 헤더, Fetch `Response`를
반환합니다.

```ts
if (result.status === 201) {
  console.log(result.headers.location);
}
```

## Link와 스트림

응답에 [Link Object](https://spec.openapis.org/oas/v3.2.0.html#link-object)가
있다면 `$links`로 후속 요청을 보낼 수 있습니다.

```ts
const created = await api.$operations.createOrder.raw({ body: order });
const receipt = await api.$links.createOrder.getReceipt(created);
```

스트리밍 API는 `$streams`에서 `AsyncIterable`로 사용할 수 있습니다.

```ts
for await (const event of api.$streams.watchOrders({
  query: { cursor: "0" },
})) {
  console.log(event);
}
```

요청 취소와 사용자 정의 전송 설정은
[전송, 인증, 스트림](./transport.md)에서 확인하세요.
