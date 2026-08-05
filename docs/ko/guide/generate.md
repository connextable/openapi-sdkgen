# SDK 생성

## 기본 명령

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api
```

- `--input`: SDK 생성에 사용할 OpenAPI JSON 또는 YAML 파일
- `--target`: 생성할 SDK 종류
- `--output`: 생성된 코드를 저장할 빈 디렉터리

TypeScript를 선택하면 클라이언트, 타입, 실행 코드, OpenAPI 메타데이터가
생성됩니다. OpenAPI 파일이 바뀌면 같은 명령을 다시 실행합니다.

## Webhook과 Callback 코드 생성

OpenAPI 파일에 Webhook 또는 Callback이 있고 애플리케이션에서 이를 받아야 한다면
`--with server`를 추가합니다.

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --with server \
  --output ./src/generated/api
```

이 옵션은 `server/webhooks.ts`와 `server/callbacks.ts`를 추가합니다. 사용법은
[Webhook과 Callback 처리](./server.md)에서 확인하세요.

## 생성 오류 확인

`generate`는 코드를 만들기 전에 OpenAPI 파일과 선택한 옵션을 검사합니다.
문제가 있으면 오류가 발생한 OpenAPI 위치와 수정에 필요한 내용을 함께
출력합니다.

경고만 있으면 생성이 계속되지만 오류가 하나라도 있으면 생성 결과를 바꾸지
않습니다. 따라서 실패한 명령 때문에 기존 생성 코드가 일부만 바뀌는 일은
없습니다.

별도의 `validate` 명령은 없습니다. 생성 가능 여부만 확인하려면 임시 디렉터리를
출력 경로로 지정하세요.

```sh
output="$(mktemp -d)/api"
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output "$output"
```

CI에서도 같은 명령의 종료 코드를 사용하면 됩니다.

::: details 원격 `$ref` 사용

원격 `$ref`는 기본적으로 가져오지 않습니다. 처음 사용할 때 허용할 HTTPS
origin을 지정하고 참조 잠금 파일을 만드세요.

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api \
  --allow-remote-ref https://schemas.example.test \
  --update-ref-lock
```

이후 생성에서는 잠금 파일에 기록된 내용과 실제 응답이 같은지 확인합니다.
`--offline`을 사용하면 이전에 저장한 참조만 사용하고 네트워크에 연결하지
않습니다.
:::

::: details 사용자 정의 JSON Schema vocabulary

OpenAPI 파일이 필수 사용자 정의 JSON Schema vocabulary를 사용한다면 저장소에
둔 확장 매니페스트를 지정할 수 있습니다.

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api \
  --schema-extension ./schema-extension.json \
  --update-ref-lock
```

확장 프로그램은 SDK를 생성할 때만 실행되며 생성된 애플리케이션 코드에는
포함되지 않습니다. 자세한 옵션은 [CLI 레퍼런스](../reference/cli.md)에서
확인하세요.
:::
