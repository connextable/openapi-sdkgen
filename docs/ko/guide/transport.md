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

## 호출 입력과 실행 환경 제어 헤더

선언된 요청 헤더는 모두 `headerParams`에 남습니다. 실행 중인 Fetch 구현이
제어하는 이름은 OpenAPI에서 필수로 선언해도 호출자 입력에서는 선택 사항으로
생성됩니다. 고정 이름과 `Proxy-*`, `Sec-*` 계열의 분류는
[Fetch 표준의 forbidden request-header 정의](https://fetch.spec.whatwg.org/#forbidden-request-header)를
따릅니다. `Origin`, `Host`, `Cookie`, `Content-Length`, `Accept-Encoding` 등이
여기에 해당합니다.

호출자는 실행 환경 제어 값을 생략할 수 있습니다.

```ts
await api.auth.oauth(provider).post({
  body: {
    intent: "login",
    returnTo,
  },
});
```

명시적으로 제공할 수도 있습니다.

```ts
await api.auth.oauth(provider).post({
  headerParams: { Origin: "https://app.example.test" },
  body: {
    intent: "login",
    returnTo,
  },
});
```

생성 런타임은 명시적인 타입 입력을 일반 `Headers` 조립 경로로 전달합니다.
선언된 매개변수가 소유하지 않는 이름은 원시 헤더로 전달할 수 있고, 헤더 기반
API key 인증 정보도 같은 방식으로 전달합니다. 선언 헤더 및 예약 헤더의 기존
소유권 충돌 검사는 유지됩니다. `X-HTTP-Method`, `X-HTTP-Method-Override`,
`X-Method-Override`는 OpenAPI의 필수 여부를 그대로 따르며, SDK에서 메서드
값을 거르지 않고 Fetch로 전달합니다.

최종 판단은 실행 중인 Fetch 구현이 합니다. 브라우저는 보안 문맥에 따라 제공된
값을 무시하거나 다시 쓰거나 직접 만들거나 거부할 수 있습니다. 주입한 Fetch
구현은 값을 허용할 수 있습니다. 따라서 생성 입력 타입은 호출자가 제공할 수
있는 값을 나타내며, 실제 전송 헤더를 보장하지 않습니다.

OpenAPI의 필수 여부는 `metadata.js`에 그대로 남습니다. 생성된 Webhook과
Callback 핸들러도 전체 인바운드 헤더 계약과 필수 검사를 유지합니다. Link의
요청 헤더 표현식은 `invocation.sourceInput`을 읽습니다. 원시 응답에는 원래
호출 입력이 자동 보관되지 않으므로 `$request.header.*`를 쓰는 Link를 따라갈
때 다시 전달해야 합니다.

```ts
const sourceInput = {
  headerParams: { Origin: "https://app.example.test" },
};
const response = await api.$operations.createSource.raw(sourceInput);

await api.$links.createSource.follow(response, { sourceInput });
```

`sourceInput`을 전달하지 않으면 소스 호출에 값을 명시했더라도 요청 헤더
표현식은 `undefined`로 해석합니다. Link가 대상 요청에 지정한 헤더는 Fetch로
그대로 전달합니다.

`headerParams.Origin`처럼 값을 이미 전달하던 호출 코드는 계속 호환되며, 이제
해당 값을 생략할 수도 있습니다. 패치 릴리스에 적합한 변경입니다.

### 사용자 정의 전송에서 헤더 정규화

신뢰할 수 있는 Node 등의 전송 구현은 SDK가 요청을 만든 다음 헤더를 추가하거나
정규화할 수도 있습니다.

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

헤더 정책이 전송 경계의 책임일 때 이 방식을 사용합니다. 최종 요청을 보내는
Fetch 구현의 제한을 우회하지는 못합니다.

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
