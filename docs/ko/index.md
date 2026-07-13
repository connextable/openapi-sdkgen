---
layout: home

hero:
  name: openapi-sdkgen
  text: OpenAPI 계약을 TypeScript 소스로 사용하세요
  tagline: 애플리케이션 코드 옆에 타입 안전한 클라이언트를 생성하고, API 변경을 더 자신 있게 배포하세요.
  actions:
    - theme: brand
      text: 3분 안에 시작하기
      link: /ko/guide/getting-started
    - theme: alt
      text: 생성된 클라이언트 살펴보기
      link: /ko/guide/client
  image:
    src: /sdk-flow.svg
    alt: OpenAPI 문서에서 TypeScript 클라이언트가 생성되는 흐름

features:
  - icon: 🧩
    title: 필요한 위치에 소스 생성
    details: 생성된 클라이언트는 애플리케이션 소스 트리에 들어갑니다. 따로 배포하거나 빌드할 SDK 패키지가 없습니다.
  - icon: ✓
    title: 경계에서 계약 검증
    details: 요청은 전송 전에 검증하고, 응답은 애플리케이션 코드가 사용하기 전에 검증합니다.
  - icon: ⚡
    title: 두 가지 방식으로 API 사용
    details: 일상적인 코드에서는 읽기 쉬운 resource를 호출하고, 필요하면 정확한 operation ID로 모든 operation에 접근합니다.
  - icon: ↗
    title: 필요할 때만 이벤트 수신
    details: 서버 add-on을 선택했을 때만 Fetch 기반 Webhook과 Callback 계약을 추가합니다.
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

출력은 상대 ESM 경로로 import하는 일반 TypeScript 소스입니다. 첫 typed request까지의 전체 흐름은
[첫 SDK 만들기](./guide/getting-started.md)에서 확인하세요.
