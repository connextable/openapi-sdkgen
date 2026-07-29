# SDK 생성

## 기본 클라이언트

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api
```

기본 출력에는 클라이언트 진입점, 생성된 선언, 런타임 헬퍼, 명시적인 메타데이터 진입점이 포함됩니다.
생성 파일에는 generated-code, lint suppression marker, Prettier의 `@noprettier` pragma가 들어갑니다.
Prettier 3.6.0 이상에서는 아래 설정으로 이 pragma를 적용할 수 있습니다.

```json
{
  "checkIgnorePragma": true
}
```

이전 Prettier를 사용한다면 `.prettierignore`에 `src/generated/**` 같은 경로를 추가하세요.

CLI는 문서의 [OpenAPI Object](https://spec.openapis.org/oas/v3.2.0.html#openapi-object)와 재사용 요소를
담은 [Components Object](https://spec.openapis.org/oas/v3.2.0.html#components-object)를 읽습니다.

## 인바운드 서버 add-on

[Callback Object](https://spec.openapis.org/oas/v3.2.0.html#callback-object)와 root `webhooks`는
애플리케이션 호스트가 소유하는 endpoint이므로, 필요한 경우에만 명시적으로 추가합니다.

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --with server \
  --output ./src/generated/api
```

이 명령은 `server/webhooks.ts`와 `server/callbacks.ts`를 추가합니다. 클라이언트 전용 루트 진입점은
그대로 유지되므로 브라우저용 import 경계도 변하지 않습니다.

대부분의 애플리케이션은 여기까지의 흐름만 따르면 됩니다. OpenAPI 문서가 바뀌면 같은 명령을 다시 실행하고,
변경된 생성 소스를 문서 변경과 함께 커밋하세요.

## 사전 진단

`generate`는 파일을 게시하기 전에 표준 OpenAPI 기본 계약과 인식하는
[SDK extension](../reference/extensions.md)을 검증합니다. 별도 `validate` command는 없습니다.

진단 보고서는 전체 error와 warning 수를 먼저 보여주고, pipeline phase와 source location별로
항목을 묶습니다. 각 항목에는 stable diagnostic code, 가능한 경우 RFC 6901 pointer,
수정 방법을 알 수 있는 message가 포함됩니다. 선행 단계가 실패하면 실행하지 못한 phase도
표시합니다. 서로 독립적인 문제는 첫 오류에서 중단하지 않고 함께 수집합니다.

Warning은 생성을 막지 않습니다. Error가 하나라도 있으면 결과를 게시하지 않습니다.
기존에 output이 없다면 새로 만들지 않고, 이미 있다면 byte 단위로 그대로 보존합니다.
예상하지 못한 내부 실패도 같은 보고서로 출력하되 credential이나 무제한 cause text를 노출하지 않습니다.

## CI

CI에서도 같은 `generate` command를 실행하고 exit status를 gate로 사용하세요. 검증만 필요하다면
temporary directory에 생성하고, 생성 source를 저장소에 커밋한다면 그 directory를 비교하거나 복사하세요.

```sh
output="$(mktemp -d)/api"
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output "$output"
```

문서와 요청한 target/add-on을 한 번에 검사하므로, 검증한 구성과 실제 게시 구성이 달라지지 않습니다.

::: details 심화: 잠긴 원격 참조

[Reference Object](https://spec.openapis.org/oas/v3.2.0.html#reference-object)의 원격 `$ref` 해석은
기본적으로 비활성화되어 있습니다. 처음 생성할 때 정확한 HTTPS origin을 허용하고
integrity lock을 의도적으로 기록해야 합니다.

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api \
  --allow-remote-ref https://schemas.example.test \
  --update-ref-lock
```

이후 실행은 lock에 기록된 응답 digest를 검증합니다. `--offline`은 인접한 `.openapi-sdkgen-cache/`만
사용하며 네트워크 연결을 열지 않습니다. 원격 URL은 HTTPS, 정확한 allowlist origin, public DNS,
제한된 redirect, 자격증명 없는 URL이라는 조건을 모두 만족해야 합니다.
:::

::: details 심화: custom JSON Schema vocabulary

[Schema Object](https://spec.openapis.org/oas/v3.2.0.html#schema-object)의 필수 custom vocabulary는
저장소에 체크인한 명시적 extension manifest를 사용합니다.

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api \
  --schema-extension ./schema-extension.json \
  --update-ref-lock
```

extension은 SDK를 생성하는 동안에만 실행됩니다. versioned JSON-RPC로 replacement JSON Schema 값을
반환하며, 생성된 애플리케이션 코드에서는 실행되지 않습니다. 모든 flag는
[CLI 레퍼런스](../reference/cli.md)에서 확인할 수 있습니다.
:::
