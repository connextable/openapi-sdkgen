# 전송, 인증, 스트림

대부분의 애플리케이션은 `baseURL`과 API 인증 정보만 설정하면 됩니다. 기본 Fetch
동작으로 충분하지 않을 때 사용자 정의 transport를 설정합니다.

## 사용자 정의 transport

다른 Fetch 구현을 사용하거나 cookie jar, 응답 헤더 접근, mTLS 같은 기능을
지정하려면 `transport`를 설정합니다.

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  transport: {
    fetch: undiciFetch,
    capabilities: {
      cookieJar: true,
      readableResponseHeaders: ["set-cookie"],
      mutualTLS: true,
    },
  },
});
```

설정한 transport가 제공하는 capability만 선언합니다.

## 요청 헤더

OpenAPI에 선언된 헤더는 `headerParams`로 전달합니다.

```ts
await api.$operations.createTodo({
  headerParams: { "Idempotency-Key": requestID },
  body: { title: "문서 작성" },
});
```

`Origin`, `Host`, `Cookie`, `Sec-*`처럼 Fetch가 제어하는 헤더는 호출자 입력에서
선택 사항입니다. 실제 전송 여부는 실행 중인 Fetch가 결정합니다.

실행 환경에서 헤더를 추가해야 한다면 사용자 정의 transport를 사용합니다.

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  transport: {
    async fetch(input, init = {}) {
      const headers = new Headers(init.headers);
      headers.set("Origin", trustedOrigin);
      return fetch(input, { ...init, headers });
    },
  },
});
```

## 인증

Bearer 토큰 하나를 사용한다면 클라이언트에 바로 지정합니다.

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  authorization: "Bearer example-token",
});
```

Operation에 OpenAPI security 대안이 여러 개라면 `securityRequirement`로 하나를
선택합니다. Requirement가 하나이면 자동으로 선택되며 빈 requirement의 이름은
`"anonymous"`입니다.

```ts
await api.$operations.updateCheckout({
  securityRequirement: "GuestCapability",
  authorization: "Bearer example-token",
});
```

선택된 requirement의 인증 정보를 가져오려면 `securityProvider`를 사용합니다.

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  securityProvider: async ({ operation, requirement, origin }) => ({
    serviceToken: {
      kind: "http-bearer",
      token: await getToken(operation, requirement, origin),
    },
  }),
});
```

API key, HTTP Basic 및 Bearer 인증, OAuth2, OpenID Connect, mTLS를 지원합니다.
로그인, 토큰 갱신, 인증 정보 저장은 애플리케이션에서 처리합니다.

브라우저에서 cookie 인증을 사용한다면 Fetch credentials를 설정합니다.

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  credentials: "include",
});
```

## 요청 취소와 시간 제한

요청 옵션에 `AbortSignal` 또는 timeout을 전달합니다.

```ts
const controller = new AbortController();

const todos = await api.todos.list(
  { query: { limit: 20 } },
  { signal: controller.signal, timeoutMS: 5_000 },
);
```

## 스트림

스트리밍 API는 `AsyncIterable`을 반환합니다. 순회를 중단하면 응답 읽기도
중단됩니다. Server-Sent Events는 자동으로 다시 연결하지 않습니다.
