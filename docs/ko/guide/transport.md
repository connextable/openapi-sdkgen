# 전송, 인증, 스트림

대부분의 애플리케이션에는 `baseURL`과, API가 요구할 때 authorization 값 또는 credential provider만 있으면
충분합니다. 일반 Fetch가 제공하지 못하는 기능이 필요한 경우에만 사용자 정의 전송을 설정하세요.

## 기본 Fetch와 사용자 정의 전송

클라이언트는 기본적으로 host `fetch`를 사용합니다. 다른 Fetch 구현을 쓰거나 추가 capability가 필요할 때만
transport를 제공하세요.

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

capability는 feature toggle이 아니라 실제 host가 제공한다는 보장입니다.
[Parameter Object](https://spec.openapis.org/oas/v3.2.0.html#parameter-object)의 호출자가 전달한 cookie parameter,
읽을 수 없는 response header, mutual TLS는 현재 환경이 제공하지 못하면 typed transport-capability
error가 발생합니다.

## Credential

토큰 획득·갱신·저장과 클라이언트 인증서 선택은 애플리케이션의 책임입니다. 생성 코드는 OpenAPI
[Security Scheme Object](https://spec.openapis.org/oas/v3.2.0.html#security-scheme-object)와
[Security Requirement Object](https://spec.openapis.org/oas/v3.2.0.html#security-requirement-object)의
wire location에 전달된 값을 적용합니다.

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

API key, HTTP basic/bearer, OAuth2/OpenID metadata, mutual-TLS capability requirement를 지원합니다.
다만 생성 SDK가 login 또는 token refresh flow를 직접 구현하지는 않습니다.

## 취소, timeout, stream

각 요청의 options는 일반 Fetch 취소 제어를 그대로 사용합니다.

```ts
const controller = new AbortController();

const todos = await api.todos.list(
  { query: { limit: 20 } },
  { signal: controller.signal, timeoutMS: 5_000 },
);
```

streaming operation은 `AsyncIterable`을 반환합니다. Fetch backpressure가 유지되며 consumer는 자연스럽게
iteration을 멈출 수 있습니다. generator는 Server-Sent Events를 자동으로 reconnect하지 않습니다.
