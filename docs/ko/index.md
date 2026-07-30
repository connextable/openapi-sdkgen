---
layout: home

hero:
  name: openapi-sdkgen
  text: OpenAPI를 애플리케이션 코드로
  tagline: API 명세와 일치하는 SDK 소스를 생성해 바로 사용하세요.
  actions:
    - theme: brand
      text: 시작하기
      link: /ko/guide/getting-started
    - theme: alt
      text: 생성된 클라이언트 살펴보기
      link: /ko/guide/client

features:
  - icon: 🧩
    title: 애플리케이션에서 바로 사용
    details: 생성한 클라이언트와 타입을 프로젝트에서 곧바로 가져와 사용합니다.
  - icon: ✓
    title: 요청과 응답 검증
    details: 요청은 전송하기 전에, 응답은 애플리케이션에서 사용하기 전에 검증합니다.
  - icon: ⚡
    title: 원하는 방식으로 API 호출
    details: 읽기 쉬운 리소스 메서드나 정확한 HTTP 메서드, 경로, operationId를 사용합니다.
  - icon: ↗
    title: Webhook과 Callback 처리
    details: 필요하면 같은 OpenAPI 문서에서 Webhook과 Callback 핸들러 타입과 라우터를 생성합니다.
---

## 명령 하나로 일반 애플리케이션 소스 생성

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api
```

```ts
import { createClient } from "./generated/api";

const api = createClient({ baseURL: "https://api.example.test/v1" });
const todo = await api.todos.create({ body: { title: "문서 작성" } });
```

명령을 실행하면 클라이언트와 타입이 지정한 디렉터리에 생성됩니다.
[첫 SDK 만들기](./guide/getting-started.md)에서 생성부터 첫 API 호출까지
따라 해보세요.
