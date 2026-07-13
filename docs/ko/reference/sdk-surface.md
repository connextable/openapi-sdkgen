# 생성 SDK 표면

생성 SDK에는 역할별 entry point가 있습니다. 작성 중인 코드의 역할에 맞는 entry만 import하세요.

## 웹 애플리케이션 진입점: `./generated/api`

클라이언트 팩토리, 리소스, 정확한 operation, 오류, stream helper, Link helper를 제공합니다.

## Metadata 진입점: `./generated/api/metadata`

tooling과 문서화에 사용할 명시적 lossless OpenAPI metadata를 제공합니다.

## 선택적 인바운드 진입점

`./generated/api/server/webhooks`는 `--with server` Webhook router와 handler type을 제공합니다.
`./generated/api/server/callbacks`는 Callback endpoint factory와 handler type을 제공합니다.

::: details Node ESM으로 직접 실행할 때

Node에서 직접 컴파일하고 실행한다면 `.js` 파일 경로를 명시해야 합니다. 예를 들어
`./generated/api/index.js`, `./generated/api/metadata.js`, 그리고 대응하는 server entry file을 사용하세요.
상대 디렉터리를 해석하는 동작은 웹 bundler의 편의 기능이며 Node ESM의 기능은 아닙니다.
:::

## Client entry

```ts
import {
  createClient,
  isTransportError,
  isValidationFailedError,
  type ClientOptions,
} from "./generated/api";
```

`createClient(options)`는 resource API와 안정적인 namespace를 반환합니다.

### Resource property

operation path에서 유도한 읽기 쉬운 resource-oriented 호출입니다.

### `$operations`

모든 visible OpenAPI `operationId`를 정확한 이름으로 제공합니다.

### `$links`

typed OpenAPI Link follow-up helper를 제공합니다.

### `$streams`

streaming response용 typed lazy `AsyncIterable` method를 제공합니다.

error는 stable code와 cause를 함께 가진 값입니다. `isTransportError`는 transport/HTTP failure를,
`isValidationFailedError`는 요청 전 또는 response decode 후에 발생한 schema validation failure를 판별합니다.

## Metadata entry

```ts
import { openapi } from "./generated/api/metadata";

openapi.document;
openapi.version;
openapi.versionLine;
```

일반 애플리케이션 import가 전체 source document에 의존하지 않도록 메타데이터는 루트 클라이언트 진입점 밖에 둡니다.

## Inbound entry

```ts
import { createWebhookRouter } from "./generated/api/server/webhooks";
import { createCallbackHandlers } from "./generated/api/server/callbacks";
```

이 entry point는 `--with server`로 생성했을 때만 존재합니다. 통합 예시는
[인바운드 서버 계약](../guide/server.md)에서 확인하세요.
