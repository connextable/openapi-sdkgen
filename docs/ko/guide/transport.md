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

## 애플리케이션 헤더와 실행 환경이 관리하는 헤더

`If-Match`, `Idempotency-Key`처럼 애플리케이션에서 설정할 수 있는 헤더는 생성
요청의 `headerParams`에 포함됩니다. 반면
[Fetch 표준](https://fetch.spec.whatwg.org/#forbidden-request-header)이 예약한
헤더는 Fetch나 실행 환경에서 관리합니다. `Origin`, `Host`, `Cookie`,
`Content-Length`, `Accept-Encoding`, 모든 `Proxy-*`, `Sec-*` 헤더가 여기에
해당합니다.

OpenAPI에서 이런 헤더를 필수로 선언해도 브라우저 호출 코드에 해당 값을 받는
입력은 생성되지 않습니다.

```ts
await api.auth.oauth(provider).post({
  body: {
    intent: "login",
    returnTo,
  },
});
```

브라우저 Fetch는 현재 보안 문맥에 따라 헤더를 추가하거나 생략하고, 필요하면
값을 다시 씁니다. API 서버는 실제로 받은 헤더를 계속 검사해야 합니다. 원본
선언은 `metadata.js`에 남고, 서버 애드온에서도 인바운드 헤더 타입과 필수
검사를 그대로 생성합니다.

`headerParams`와 원시 `headers` 옵션으로는 실행 환경이 관리하는 헤더를 넣을 수
없습니다. JavaScript나 `as any`로 타입을 우회해도 전송 전에 차단됩니다.
`X-HTTP-Method`, `X-HTTP-Method-Override`, `X-Method-Override`는 입력으로
사용할 수 있지만, 직렬화한 값에 `CONNECT`, `TRACE`, `TRACK`이 포함되면
차단됩니다.

기존 SDK를 다시 생성한 뒤에는 `headerParams.Origin`처럼 직접 전달하던 값을
호출 코드에서 제거해야 합니다. 이 변경은 의도적인 소스 호환성 변경이며, 이전
속성을 유지하는 호환 별칭은 생성하지 않습니다.

### 브라우저가 아닌 전송에서 헤더 추가

신뢰할 수 있는 Node 등의 전송 구현은 SDK가 요청을 만든 다음 실행 환경이
관리하는 헤더를 추가할 수 있습니다. 이 값은 operation 입력이나 원시 헤더
옵션으로 받지 마세요.

```ts
const nodeFetch = globalThis.fetch;

const api = createClient({
  baseURL: "https://api.example.test",
  transport: {
    async fetch(input, init = {}) {
      const headers = new Headers(init.headers);
      headers.set("Origin", trustedOrigin);
      return nodeFetch(input, { ...init, headers });
    },
  },
});
```

이 방식은 전송 구현이 해당 값을 소유하는 신뢰 경계일 때만 사용하세요. 브라우저
Fetch의 제한을 우회하지는 못합니다.

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
