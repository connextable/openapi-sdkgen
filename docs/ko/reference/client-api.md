# 생성된 클라이언트 API

TypeScript SDK는 용도에 따라 가져올 경로가 나뉩니다. 일반적인 API 호출에는
`./generated/api`만 사용하면 됩니다.

| 경로 | 용도 |
| --- | --- |
| `./generated/api` | API 호출, 생성 타입, 오류, Link, 스트림 |
| `./generated/api/metadata` | 원본 OpenAPI 파일과 버전 확인 |
| `./generated/api/server/webhooks` | Webhook 처리 |
| `./generated/api/server/callbacks` | Callback 처리 |

::: details Node ESM으로 직접 실행할 때

Node에서 컴파일된 파일을 직접 실행한다면 `.js` 파일 경로를 명시하세요.

```ts
import { createClient } from "./generated/api/index.js";
```

Node ESM은 디렉터리 경로에서 `index.js`를 자동으로 찾지 않습니다.
:::

## 클라이언트

```ts
import { createClient } from "./generated/api";

const api = createClient({
  baseURL: "https://api.example.test/v1",
});
```

클라이언트 설정은 [전송, 인증, 스트림](../guide/transport.md)에서 확인하세요.

## 생성 타입

```ts
import {
  Enums,
  type Client,
  type ComponentInput,
  type ComponentOutput,
  type Components,
  type LinkCalls,
  type OperationBody,
  type OperationContract,
  type OperationHeaders,
  type OperationInput,
  type OperationOutput,
  type OperationParameter,
  type OperationPath,
  type OperationQuery,
  type Operations,
  type PaginateCall,
  type RawCall,
  type ResourceCall,
  type RouteBody,
  type RouteContract,
  type RouteInput,
  type RouteOutput,
  type RouteParameter,
  type RouteQuery,
  type Routes,
  type StreamCall,
} from "./generated/api";
```

- `Components`: OpenAPI의 컴포넌트 스키마와 입출력 타입
- `Enums`: 컴포넌트 enum의 실제 값
- `Routes`: HTTP 메서드와 OpenAPI 경로로 찾는 모든 API 타입
- `Operations`: `operationId`로 찾는 API 타입
- `RouteContract<Route>`: 정확한 route 하나의 전체 계약
- `OperationContract<Source>`: operation ID나 생성된 메서드 타입으로 선택한 계약
- `OperationInput<Source>`, `OperationOutput<Source>`: 해당 호출 표면이 받는 입력과
  디코딩된 출력
- `OperationBody`, `OperationQuery`, `OperationPath`, `OperationHeaders`,
  `OperationCookies`, `OperationQuerystring`: 재사용할 수 있는 요청 영역
- `OperationParameter<Source, Location, Name>`: path, query, query-string, header,
  cookie 중 하나의 파라미터 값
- `RouteInput`, `RouteOutput`, `RouteBody`, `RouteQuery`, `RoutePath`,
  `RouteHeaders`, `RouteCookies`, `RouteQuerystring`, `RouteParameter`: 정확한 route
  문자열로 찾는 대응 helper
- `ResourceCall<Route>`: 리소스 메서드에 사용되는 호출 타입
- `RawCall<Route>`, `PaginateCall<Route>`, `LinkCalls<Route>`,
  `StreamCall<Route>`: route별 기능 호출 타입

```ts
type MoneyInput = Components["Money"]["input"];
type MoneyOutput = Components["Money"]["output"];
type CanonicalMoneyInput = ComponentInput<"Money">;
type CanonicalMoneyOutput = ComponentOutput<"Money">;
type ListPetsInput = Routes["GET /pets"]["input"];
type GetPetInput = Operations["get-pet"]["input"];
type CreateOrder = RouteContract<"POST /orders">;
type CreateOrderInput = RouteInput<"POST /orders">;
type CreateOrderOutput = RouteOutput<"POST /orders">;
type CreateOrderCall = ResourceCall<"POST /orders">;
type CreateOrderRawCall = RawCall<"POST /orders">;
const firstCurrency = Enums["Currency"][0];
```

한 route의 여러 계약 슬롯을 함께 다룬다면 `RouteContract<Route>`를 사용합니다.
입력, 출력, 요청 영역, 파라미터 값 하나만 필요하다면 대응하는 `Route*` helper를
사용합니다. route가 해당 기능을 지원하지 않으면 기능 슬롯과 대응 helper 타입은
`never`가 됩니다.

### Operation 요청 타입 추출

모든 `Operation*<Source>` helper는 다음 source 형식을 받습니다.

| Source | 선택되는 입력 |
| --- | --- |
| 정확한 `operationId` 문자열 | path 파라미터를 포함한 전체 입력 |
| 생성된 `$operations` 메서드 | path 파라미터를 포함한 전체 입력 |
| 생성된 `$routes` 메서드 | path 파라미터를 포함한 전체 입력. `operationId` 불필요 |
| 중첩된 리소스 트리의 leaf 메서드 | 이미 바인딩된 path 파라미터를 제거한 호출 입력 |

```ts
type ByID = OperationInput<"createAfterSalesRequest">;
type ByOperationMethod = OperationInput<
  Client["$operations"]["createAfterSalesRequest"]
>;
type ByRouteMethod = OperationInput<
  Client["$routes"]["POST /orders/{orderID}/after-sales-requests"]
>;

declare const createFromTree:
  ReturnType<Client["orders"]>["afterSalesRequests"]["create"];

type BoundInput = OperationInput<typeof createFromTree>;
type Created = OperationOutput<typeof createFromTree>;
type Body = OperationBody<typeof createFromTree>;
```

`ByID`, `ByOperationMethod`, `ByRouteMethod`에는 `path`와 `body`가 들어 있습니다.
`BoundInput`에는 `body`만 있고 `path`는 없습니다. 리소스 트리가 이미 `orderID`를
받았기 때문입니다. 정확한 route 문자열 자체를 타입 식별자로 사용하려면
`RouteInput<"METHOD /path">`를 사용합니다. route 문자열은 operation ID가 아니므로
`Operation*`의 문자열 source로 사용할 수 없습니다. 같은 이유로
`OperationPath<typeof createFromTree>`도 거부됩니다. 해당 호출에는 남은 path
입력이 없습니다.

요청 영역과 개별 파라미터 helper를 사용하면 소비자 코드에 `NonNullable`을
반복해서 작성할 필요가 없습니다.

```ts
type Filters = OperationQuery<"listAfterSalesRequests">;
type State = OperationParameter<
  "listAfterSalesRequests",
  "query",
  "state"
>;
type ApprovalBody = OperationBody<"approveAfterSalesRequest">;
type ApprovalHeaders = OperationHeaders<"approveAfterSalesRequest">;

type RouteFilters = RouteQuery<"GET /after-sales-requests">;
type RouteState = RouteParameter<
  "GET /after-sales-requests",
  "query",
  "state"
>;
```

영역 helper는 aggregate 입력 필드가 선택 사항이어서 생긴 바깥쪽 `undefined`만
제거합니다. 영역 내부의 선택 속성은 그대로 선택 사항입니다. 개별 파라미터
helper는 생략을 뜻하는 `undefined`만 제거하고 스키마에 선언된 `null`은
보존합니다. 전체 호출에서 해당 영역을 생략할 수 있는지는 `OperationInput`이나
`RouteInput`을 기준으로 판단합니다.

OpenAPI에 선언된 이름은 철자와 대소문자를 그대로 사용합니다. 하이픈처럼
TypeScript 속성 이름으로 바로 쓸 수 없는 문자가 있다면 대괄호 표기법을
사용하세요.

## API 호출

### 리소스 메서드

일반적인 애플리케이션 코드에서는 경로를 바탕으로 생성된 리소스 메서드를
사용합니다.

```ts
const todo = await api.todos.create({
  body: { title: "문서 작성" },
});
```

### `$routes`

HTTP 메서드와 OpenAPI 경로를 그대로 사용합니다. `operationId`가 없는 API도
호출할 수 있습니다.

```ts
const todos = await api.$routes["GET /todos"]({
  query: { limit: 20 },
});
```

### `$operations`

OpenAPI에 선언된 `operationId`로 호출합니다.

```ts
const todos = await api.$operations["listTodos"]({
  query: { limit: 20 },
});
```

## Security Requirement

operation에 OpenAPI Security Requirement Object가 선언되어 있으면 생성된 요청
옵션은 적용되는 requirement가 둘 이상일 때만 `securityRequirement`를
노출합니다. 이 속성은 해당 operation의 안정적인 requirement ID로 구성된 필수
유니언이며 operation options 인자도 필수입니다. 빈 `{}` requirement는
`"anonymous"`로 표현됩니다. 적용되는 requirement가 정확히 하나이면 SDK가
자동으로 선택합니다. requirement가 없거나 하나인 operation은 selector를
노출하지 않습니다.

```ts
await api.$operations.updateCheckout({
  securityRequirement: "GuestCapability",
  authorization: "Bearer example-token",
});
```

`ClientOptions.credentials`와 요청별 `credentials`는 Fetch의
`RequestCredentials`입니다. 요청에서 선택한 requirement의 인증 정보를
가져오려면 `ClientOptions.securityProvider`를 사용합니다. provider는 선택된
단일 `requirement`를 받고 scheme별 인증 정보 맵을 반환하며 requirement를
선택할 수 없습니다. 모호한 선택을 생략하면 provider와 Fetch를 호출하기 전에
실패합니다. 자세한 내용은
[Security Requirement 선택과 인증 정보 제공](../guide/transport.md#security-requirement-선택과-인증-정보-제공)을
참고하세요.

## 요청 헤더

선언된 헤더는 모두 `headerParams`에 생성됩니다. `Origin`, `Host`, `Cookie`,
`Sec-*`처럼 Fetch가 제어하는 헤더는 OpenAPI에서 필수로 선언해도 호출자
입력에서는 선택 사항입니다. 명시적인 타입 입력, 선언되지 않은 원시 헤더,
헤더 기반 API key 인증 정보는 실행 중인 Fetch 구현으로 전달합니다. 선언 및
예약 헤더와 원시 헤더 사이의 소유권 충돌은 계속 오류입니다.

Method override 헤더는 OpenAPI의 필수 여부를 유지하며 SDK의 값 필터 없이
Fetch로 전달됩니다. Link는 명시적으로 전달한 `invocation.sourceInput`에서
요청 헤더 소스를 읽습니다. 소스 호출 입력은 자동 보관되지 않습니다. 대상
값은 같은 경로로 전달합니다.

원본 필수 여부는 `openapi.document`에 남습니다. 생성된 Webhook과 Callback
서버 애드온 입력 타입 및 런타임 검사는 전체 인바운드 계약을 유지합니다.

최종 Fetch 동작과 사용자 정의 전송 경계는
[호출 입력과 실행 환경 제어 헤더](../guide/transport.md#호출-입력과-실행-환경-제어-헤더)에서
확인하세요.

## Link와 스트림

- `$links`: OpenAPI Link에 정의된 후속 요청
- `$streams`: 스트리밍 응답을 읽는 `AsyncIterable`

사용 예시는 [생성된 클라이언트 사용](../guide/client.md)에서 확인하세요.

## 오류 처리

```ts
import {
  isAPIError,
  isErrorCategory,
  isErrorCode,
  TransportErrorCode,
} from "./generated/api";
```

- `isAPIError(error)`: 생성된 API 오류인지 확인
- `isErrorCode(error, code)`: 정확한 오류 코드 확인
- `isErrorCategory(error, category)`: 오류 범주 확인
- `TransportErrorCode`: 전송 과정에서 발생할 수 있는 오류 코드

Security Requirement 선택 오류는 `SECURITY_REQUIREMENT_REQUIRED`와
`SECURITY_REQUIREMENT_INVALID`를 사용합니다. 인증 정보 획득 및 적용 오류는
`SECURITY_CREDENTIALS_REQUIRED`와 `SECURITY_CREDENTIALS_INVALID`를 사용합니다.

## OpenAPI 메타데이터

```ts
import { openapi } from "./generated/api/metadata";

openapi.document;
openapi.version;
openapi.versionLine;
```

`openapi.document`에서 SDK 생성에 사용한 OpenAPI 파일의 내용을 확인할 수
있습니다.

## Webhook과 Callback

`--with server`로 생성했다면 다음 경로를 사용할 수 있습니다.

```ts
import { createWebhookRouter } from "./generated/api/server/webhooks";
import { createCallbackHandlers } from "./generated/api/server/callbacks";
```

자세한 사용법은
[Webhook과 Callback 처리](../guide/server.md)에서 확인하세요.
