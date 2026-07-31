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

## Security Requirement 선택과 인증 정보 제공

간단한 Bearer 토큰은 클라이언트를 만들 때 바로 지정할 수 있습니다.

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  authorization: "Bearer example-token",
});
```

OpenAPI는 `security` 배열의 각 객체를 Security Requirement Object라고
부릅니다. 객체 사이는 OR이고, 한 객체 안의 scheme은 모두 함께 충족해야
합니다(AND). 생성된 operation 옵션은 각 객체의 안정적인 ID를 정확한
`securityRequirement` 유니언으로 제공합니다. 적용되는 requirement가 둘 이상이면
이 속성과 operation의 options 인자가 필수입니다. 빈 requirement의 안정적인 ID는
`"optional"`이며 다른 requirement가 함께 있으면 익명 요청도 명시적으로
선택해야 합니다.

```ts
await api.$operations.updateCheckout({
  securityRequirement: "GuestCapability",
  authorization: "Bearer example-token",
});

await api.$operations.startOAuth(input, {
  securityRequirement: "optional",
});
```

선택된 requirement에 필요한 인증 정보를 호스트에서 가져오려면
`securityProvider`를 사용합니다. `requirements`에는 operation에 적용되는
Security Requirement Object가 들어 있습니다. requirement가 둘 이상이면 호출자가
선택한 값이 `selectedRequirement`로 전달됩니다. 적용되는 requirement가 정확히
하나이고 호출자가 선택을 생략한 경우에만 provider가 그 단일 requirement를
선택할 수 있습니다.

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  securityProvider: async ({ operation, requirements, selectedRequirement, origin }) => {
    const requirement = selectedRequirement ?? requirements.serviceToken;
    return {
      requirement,
      credentials: {
        serviceToken: {
          kind: "http-bearer",
          token: await getToken(operation, origin),
        },
      },
    };
  },
});
```

API key, HTTP Basic/Bearer, OAuth2, OpenID Connect, mTLS 요구 사항을 지원합니다.
로그인, 토큰 갱신, 인증 정보 저장은 애플리케이션에서 구현해야 합니다.

적용되는 requirement가 둘 이상이면 `securityRequirement`를 반드시 명시해야
합니다. 생략하면 `securityProvider`나 Fetch를 호출하기 전에
`SECURITY_REQUIREMENT_REQUIRED`로 실패합니다. 적용되는 requirement가 정확히
하나이면 selector는 선택 사항이며 provider를 사용하거나 SDK 소유 옵션이 단일
requirement를 이미 충족할 때 자동 추론할 수 있습니다. 선택한 scheme과 일치하는
`authorization` 또는 `csrfToken`은 인증 정보를 충족합니다. provider는 해당
scheme을 생략하거나 같은 값을 반환할 수 있습니다. 다른 값을 반환하면 Fetch
전에 실패합니다. 원시 `headers`는 security scheme을 충족하지 않습니다.

업그레이드 후 client를 다시 생성해야 합니다. requirement가 둘 이상인 기존
호출은 명시적인 selector를 전달할 때까지 TypeScript 오류가 됩니다. 익명 요청은
`"optional"`을 전달합니다.

OpenAPI cookie API key security scheme은 브라우저의 ambient cookie로 충족할 수
있습니다. JavaScript에서 쿠키 값을 읽을 필요가 없습니다.

```ts
const api = createClient({
  baseURL: "https://api.example.test",
  credentials: "include",
});
```

`credentials`는 Fetch의 `RequestCredentials` 정책만 나타냅니다.
`securityProvider`와 함께 설정할 수 있으며, ambient cookie는 여러 requirement
중 하나를 선택하지 않습니다. requirement를 선택한 뒤 `"include"`를 사용하면
cookie security는 실행 중인 Fetch 구현에 맡깁니다. SDK는 쿠키 값을 요구하거나
`Cookie` 헤더를 만들지 않습니다. 실제 전송 여부는 Fetch와 브라우저 쿠키
정책이 결정합니다.

세션 cookie와 CSRF header를 함께 요구하는 requirement는 다음처럼 선택하고 두
scheme을 전용 요청 옵션으로 충족할 수 있습니다.

```ts
await api.$operations.updateCheckout({
  securityRequirement: "BuyerCSRFHeader__BuyerSessionCookie",
  credentials: "include",
  csrfToken: csrf,
});
```

선택 오류는 `SECURITY_REQUIREMENT_REQUIRED` 또는
`SECURITY_REQUIREMENT_INVALID`를 사용합니다. 누락되거나 잘못된 인증 정보,
추가 인증 정보, 충돌은 `SECURITY_CREDENTIALS_REQUIRED` 또는
`SECURITY_CREDENTIALS_INVALID`를 사용합니다.

같은 cookie를 `in: cookie` Parameter Object와 적용된 cookie security scheme에
동시에 선언하면 소유권이 모호하므로 생성을 거부합니다. 일반 cookie parameter는
계속 명시적인 호출자 입력이며 cookie-jar capability가 있는 transport가
필요합니다.

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
