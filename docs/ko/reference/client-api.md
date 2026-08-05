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

## TypeScript 타입

생성된 SDK는 component, route, operation, 요청 영역, 파라미터를 기준으로 타입을
제공합니다. 전체 타입 API와 예제는
[생성된 TypeScript 타입](./typescript-types.md)에서 확인하세요.

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
예시는 [인증](../guide/transport.md#인증)에서 확인할 수 있습니다.

## 요청 헤더

선언된 헤더는 모두 `headerParams`에 생성됩니다. Fetch가 제어하는 헤더는 호출자
입력에서 선택 사항이며 실제 전송 여부는 실행 중인 Fetch가 결정합니다. 자세한
사용법은 [요청 헤더](../guide/transport.md#요청-헤더)에서 확인할 수 있습니다.

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
