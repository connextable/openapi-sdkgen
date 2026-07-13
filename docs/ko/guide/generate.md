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
