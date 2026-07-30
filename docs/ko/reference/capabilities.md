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

요청 헤더는 애플리케이션에서 설정할 수 있는 값과 Fetch가 관리하는 값을
구분합니다. Fetch 관리 헤더 선언은 메타데이터와 서버의 인바운드 계약에는
남지만, 클라이언트 입력에서는 빠지고 원시 헤더 옵션에서도 차단됩니다. 값에
따라 제한되는 method override 헤더에는 Fetch의 금지 메서드 규칙을 적용합니다.
자세한 내용은
[애플리케이션 헤더와 실행 환경이 관리하는 헤더](../guide/transport.md#애플리케이션-헤더와-실행-환경이-관리하는-헤더)에서
확인하세요.

## 서버와 인증

OpenAPI의 Server Object, 보안 스키마, operation별 보안 설정을 지원합니다.
토큰과 인증서는 애플리케이션에서 제공해야 합니다.

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
