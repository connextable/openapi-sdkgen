# OpenAPI 지원 범위

openapi-sdkgen은 OpenAPI 3.0.x, 3.1.x, 3.2.x 파일을 지원합니다. 파일에 선언된
OpenAPI 버전에 맞춰 각 필드를 해석합니다.

## 요청과 응답

다음 내용을 TypeScript 타입과 클라이언트 코드에 반영합니다.

- API 경로, HTTP 메서드, 매개변수
- JSON, 텍스트, 바이너리, form, multipart 요청 본문
- 상태 코드별 응답과 응답 헤더
- OpenAPI 스키마에 따른 요청·응답 검사

생성된 클라이언트에서는 리소스 메서드, 정확한 HTTP 경로,
`operationId` 가운데 원하는 방식으로 API를 호출할 수 있습니다.

요청 헤더는 호출자 필수 여부와 실제 전송 필수 여부를 구분합니다. Fetch가
제어하는 이름도 명시적으로 설정할 수 있지만 호출자 입력에서는 선택 사항입니다.
타입 입력, 선언되지 않은 원시 헤더, 헤더 기반 API key, method override 값은
SDK 정책으로 차단하지 않고 Fetch로 전달합니다. 메타데이터와 생성된 Webhook 및
Callback 계약은 OpenAPI의 원래 필수 여부를 유지합니다. 자세한 내용은
[호출 입력과 실행 환경 제어 헤더](../guide/transport.md#호출-입력과-실행-환경-제어-헤더)에서
확인하세요.

## 서버와 인증

OpenAPI Server Object, security scheme, operation별 Security Requirement Object를
지원합니다. 생성된 operation 옵션은 안정적인 ID를 `securityRequirement`로
제공합니다. 토큰과 인증서는 애플리케이션에서 직접 제공하거나
`securityProvider`로 가져옵니다.

지원하는 인증 방식과 설정 방법은
[전송, 인증, 스트림](../guide/transport.md)에서 확인하세요.

## Link와 스트림

OpenAPI Link는 후속 요청을 보내는 타입 안전 도우미로 생성됩니다. 스트리밍
응답은 `AsyncIterable`로 읽을 수 있습니다.

## Webhook과 Callback

`--with server`를 추가하면 Webhook과 Callback을 받는 데 필요한 타입과 Fetch
기반 라우터를 생성합니다.

## 지원하지 않는 기능

OpenAPI 파일에 현재 생성할 수 없는 기능이 있으면 해당 위치를 알려 주는 오류와
함께 생성을 중단합니다. 지원하지 않는 내용을 조용히 건너뛰지는 않습니다.

원본 OpenAPI 정보 가운데 클라이언트 API로 표현하지 않는 내용은
`metadata.js`에서 확인할 수 있습니다.
