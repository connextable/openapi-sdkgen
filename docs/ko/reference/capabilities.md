# OpenAPI 지원 범위

이 페이지는 OpenAPI 문서를 그대로 생성할 수 있는지 판단하거나, 특정 기능의 버전별 지원 여부를 자세히
확인해야 할 때 사용하세요. 일반적인 SDK 생성과 사용을 위해 구현 matrix를 먼저 읽을 필요는 없습니다.

가이드의 문법 링크는 최신
[OpenAPI 3.2 Object reference](https://spec.openapis.org/oas/v3.2.0.html)를 가리킵니다. generator는
각 문서에 선언된 3.0, 3.1, 3.2 버전에 맞춰 해석하므로, 버전 전용 기능은 해당
[3.0](https://spec.openapis.org/oas/v3.0.4.html) 또는
[3.1](https://spec.openapis.org/oas/v3.1.1.html) 명세도 함께 확인하세요.

## 애플리케이션에서 사용할 수 있는 기능

### 표준 요청과 응답 형태

HTTP operation, parameter, JSON/text/binary/form/multipart body, typed response를 사용하면 resource 호출과
정확한 `operationId` 호출을 갖는 TypeScript 클라이언트가 생성됩니다.

### Server, security, Link, stream

OpenAPI Server, security scheme, response header, Link, stream은 클라이언트 동작으로 생성됩니다. 플랫폼이
담당해야 하는 전송과 credential은 호스트에서 제공합니다.

### Callback과 Webhook

`--with server`를 추가하면 Fetch 기반 인바운드 handler 계약이 생성됩니다.

### OpenAPI 버전

OpenAPI 3.0.x, 3.1.x, 3.2.x는 문서에 선언된 버전에 맞춰 파싱하고 생성합니다.

## 생성이 중단되는 경우

generator는 유효한 construct를 조용히 건너뛰지 않습니다. 선택한 target 또는 add-on이 해당 기능을
표현할 수 없다면 feature-path diagnostic과 함께 generation을 중단합니다. 일부 원본 정보는 임의의
runtime API를 만들어 내는 대신 `metadata.js`에서만 의도적으로 제공합니다.

## 세부 구현 근거

아래 문서는 compiler와 test와 함께 관리되는 영문 원본입니다. 감사, migration, 특정 기능의 정확한
호환성 확인에 사용하세요. 큰 표는 좁은 화면에서도 본문 영역 안에서 가로로 스크롤됩니다.

- [Capability inventory](https://github.com/connextable/openapi-sdkgen/blob/main/docs/openapi-feature-inventory.md): 그룹별 기능 목록입니다.
- [Capability matrix](https://github.com/connextable/openapi-sdkgen/blob/main/docs/openapi-feature-matrix.md): 버전별 상태와 test 근거를 제공합니다.
- [Feature manifest](https://github.com/connextable/openapi-sdkgen/blob/main/docs/openapi-feature-manifest.json): machine-readable source of truth입니다.
- [Implementation roadmap](https://github.com/connextable/openapi-sdkgen/blob/main/docs/openapi-sdk-implementation-roadmap.md): architecture와 구현 계약을 설명합니다.
