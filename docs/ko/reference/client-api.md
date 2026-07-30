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
  type Components,
  type Operations,
  type Routes,
} from "./generated/api";
```

- `Components`: OpenAPI의 컴포넌트 스키마와 입출력 타입
- `Enums`: 컴포넌트 enum의 실제 값
- `Routes`: HTTP 메서드와 OpenAPI 경로로 찾는 모든 API 타입
- `Operations`: `operationId`로 찾는 API 타입

```ts
type MoneyInput = Components["Money"]["input"];
type MoneyOutput = Components["Money"]["output"];
type ListPetsInput = Routes["GET /pets"]["input"];
type GetPetInput = Operations["get-pet"]["input"];
const firstCurrency = Enums["Currency"][0];
```

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

## 요청 헤더

선언된 헤더는 모두 `headerParams`에 생성됩니다. `Origin`, `Host`, `Cookie`,
`Sec-*`처럼 Fetch가 제어하는 헤더는 OpenAPI에서 필수로 선언해도 호출자
입력에서는 선택 사항입니다. 명시적인 타입 입력, 선언되지 않은 원시 헤더,
헤더 기반 API key 인증 정보는 실행 중인 Fetch 구현으로 전달합니다. 선언 및
예약 헤더와 원시 헤더 사이의 소유권 충돌은 계속 오류입니다.

Method override 헤더는 OpenAPI의 필수 여부를 유지하며 SDK의 값 필터 없이
Fetch로 전달됩니다. Link는 원래 호출 입력에서 요청 헤더 소스를 읽고, 대상
값도 같은 경로로 전달합니다.

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
