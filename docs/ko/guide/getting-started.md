# 시작하기

`openapi-sdkgen`은 유효한 OpenAPI 3.0.x, 3.1.x, 3.2.x 문서를 일반 TypeScript
소스로 변환합니다. 생성된 파일은 별도 npm 패키지가 아니라 애플리케이션 소스의 일부로 관리됩니다.

입력 문서는 [OpenAPI Object](https://spec.openapis.org/oas/v3.2.0.html#openapi-object)이며,
그 안의 [Paths Object](https://spec.openapis.org/oas/v3.2.0.html#paths-object)와
[Operation Object](https://spec.openapis.org/oas/v3.2.0.html#operation-object)가 생성될 호출을 설명합니다.

## 1. CLI 설치

일반 Node 기반 애플리케이션 프로젝트에서는 사전 컴파일된 npm CLI를 사용합니다. 플랫폼별 실행 파일이
포함되어 있으므로 소비자에게 Go는 필요하지 않습니다.

```sh
pnpm dlx openapi-sdkgen generate \
  --input ./openapi.yaml \
  --target typescript \
  --output ./src/generated/api
```

GitHub Release binary를 직접 사용해도 됩니다. Go 사용자는 모듈에서
동일한 CLI를 설치할 수도 있습니다.

```sh
go install github.com/connextable/openapi-sdkgen/cmd/openapi-sdkgen@latest
```

macOS 또는 Linux에서는 Go를 따로 준비하지 않고 Homebrew로 설치할 수 있습니다.

```sh
brew install connextable/tap/openapi-sdkgen
```

## 2. 앱 소스에 생성

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api
```

출력 디렉터리는 비어 있어야 합니다. 생성기는 모든 산출물을 staging 영역에 먼저 작성하고, 모든 단계가
성공했을 때만 최종 디렉터리에 publish합니다.

루트 문서는 `file://` URL, HTTP(S) development server, stdin에서도 받을 수 있습니다. 이는 remote `$ref`가
아닌 입력 source입니다.

```sh
openapi-sdkgen generate \
  --input http://localhost:4010/openapi.json \
  --target typescript \
  --output ./src/generated/api
```

다른 명령이 문서를 출력한다면 `--input -`를 사용하세요. 문서에 상대 `$ref`가 있다면
`--input-base <path-or-url>`를 추가합니다.

## 3. 웹 애플리케이션에서 클라이언트 import

```ts
import { createClient } from "./generated/api";

const api = createClient({
  baseURL: "https://api.example.test/v1",
});
```

Vite, Next.js, Nuxt 같은 웹 bundler는 생성 디렉터리를 `index.ts` 진입점으로 해석합니다. 생성 패키지의
manifest를 만들거나 소비자 프로젝트에서 SDK 전용 빌드를 수행할 필요가 없습니다.

::: details Node ESM으로 직접 실행할 때

Node ESM은 상대 디렉터리를 `index.js`로 자동 해석하지 않습니다. 애플리케이션을 Node에서 직접 컴파일하고
실행한다면 `./generated/api/index.js`를 import하세요.
:::

## 4. 생성 resource 호출

```ts
const todo = await api.todos.create({
  body: { title: "읽기 쉬운 코드" },
});

const page = await api.todos.list({
  query: { limit: 20 },
});
```

resource 호출은 애플리케이션 코드에서 읽기 쉬운 API를 제공합니다. 모든 operation은 정확한
`operationId` 이름으로 `api.$operations`에서도 접근할 수 있습니다.

다음 단계로 [SDK 생성](./generate.md)과 [클라이언트 사용](./client.md)을 확인하세요.
