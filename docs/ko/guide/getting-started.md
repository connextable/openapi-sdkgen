# 시작하기

`openapi-sdkgen`은 OpenAPI 3.x 파일로 애플리케이션에서 사용할 SDK 소스를
만드는 CLI입니다. 이 가이드에서는 OpenAPI JSON 또는 YAML 파일로 TypeScript
클라이언트를 만들고 첫 API 요청까지 직접 실행합니다.

## 1. CLI 실행

Node.js 프로젝트에서는 npm 패키지로 CLI를 바로 실행할 수 있습니다.

```sh
pnpm dlx openapi-sdkgen generate \
  --input ./openapi.yaml \
  --target typescript \
  --output ./src/generated/api
```

macOS와 Linux에서는 Homebrew로 설치할 수도 있습니다.

```sh
brew install jinyongp/tap/openapi-sdkgen
```

GitHub Releases에서도 운영체제별 실행 파일을 내려받을 수 있습니다.

## 2. OpenAPI 파일에서 클라이언트 생성

`--input`에는 SDK 생성에 사용할 OpenAPI JSON 또는 YAML 파일을 지정합니다.
`--output`에는 생성된 코드를 저장할 빈 디렉터리를 지정합니다.

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api
```

생성이 끝나면 `./src/generated/api` 아래에 클라이언트와 타입이 만들어집니다.

URL에 있는 OpenAPI 파일을 바로 사용할 수도 있습니다.

```sh
openapi-sdkgen generate \
  --input http://localhost:4010/openapi.json \
  --target typescript \
  --output ./src/generated/api
```

다른 명령이 출력한 OpenAPI 파일을 읽으려면 `--input -`를 지정합니다. 여기서
`-`는 파일 경로 대신 표준 입력(stdin)을 사용한다는 뜻입니다. OpenAPI 파일이
상대 `$ref`를 사용한다면 `--input-base`로 기준 경로 또는 URL을 함께
지정하세요.

## 3. 클라이언트 만들기

```ts
import { createClient } from "./generated/api";

const api = createClient({
  baseURL: "https://api.example.test/v1",
});
```

Vite, Next.js, Nuxt 같은 번들러는 생성 디렉터리의 `index.ts`를 자동으로
찾습니다.

::: details Node ESM으로 직접 실행할 때

Node ESM은 디렉터리 경로에서 `index.js`를 자동으로 찾지 않습니다. Node에서
컴파일된 파일을 직접 실행한다면 `./generated/api/index.js`에서 가져오세요.
:::

## 4. API 호출

OpenAPI 파일에 정의된 API 경로와 HTTP 메서드를 바탕으로 리소스 메서드가
생성됩니다.

```ts
const todo = await api.todos.create({
  body: { title: "문서 작성" },
});

const todos = await api.todos.list({
  query: { limit: 20 },
});
```

모든 API는 HTTP 메서드와 OpenAPI 경로를 사용해 `api.$routes`에서도 호출할 수
있습니다. `operationId`가 선언되어 있다면 `api.$operations`도 사용할 수
있습니다.

다음으로 [SDK 생성 옵션](./generate.md)과
[생성된 클라이언트 사용법](./client.md)을 확인하세요.
