# 전송, 인증, 스트림

대부분의 애플리케이션은 `baseURL`과 API 인증 정보만 설정하면 됩니다. 기본
Fetch로 처리할 수 없는 기능이 있을 때만 사용자 정의 전송을 사용하세요.

## 사용자 정의 전송

클라이언트는 실행 환경의 `fetch`를 기본으로 사용합니다. 다른 Fetch 구현이
필요하거나 쿠키 저장소, 응답 헤더 접근, mTLS 같은 기능을 사용해야 한다면
`transport`를 설정합니다.

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

`capabilities`에는 실제 실행 환경에서 지원하는 기능만 지정해야 합니다. 필요한
기능이 없으면 요청을 보내기 전에 오류가 발생합니다.

## 인증 정보 제공

간단한 Bearer 토큰은 클라이언트를 만들 때 바로 지정할 수 있습니다.

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  authorization: "Bearer example-token",
});
```

여러 인증 방식을 선택하거나 토큰을 요청할 때마다 가져와야 한다면
`credentials` 함수를 사용합니다.

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  credentials: async ({ alternatives }) => {
    const alternative = alternatives.serviceToken;
    return {
      alternative,
      values: {
        serviceToken: { kind: "http-bearer", token: await getToken() },
      },
    };
  },
});
```

API key, HTTP Basic/Bearer, OAuth2, OpenID Connect, mTLS 요구 사항을 지원합니다.
로그인, 토큰 갱신, 인증 정보 저장은 애플리케이션에서 구현해야 합니다.

## 요청 취소와 시간 제한

각 요청에 `AbortSignal`과 시간 제한을 지정할 수 있습니다.

```ts
const controller = new AbortController();

const todos = await api.todos.list(
  { query: { limit: 20 } },
  { signal: controller.signal, timeoutMS: 5_000 },
);
```

## 스트림

스트리밍 API는 `AsyncIterable`을 반환합니다. 순회를 중단하면 응답 읽기도
중단됩니다. Server-Sent Events 연결이 끊어졌을 때 자동으로 다시 연결하지는
않습니다.
