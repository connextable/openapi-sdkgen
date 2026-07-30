# CLI 레퍼런스

## 도움말과 버전

```sh
openapi-sdkgen --help
openapi-sdkgen generate --help
openapi-sdkgen --version
```

`openapi-sdkgen --help`는 사용할 수 있는 명령을 보여줍니다.
`openapi-sdkgen generate --help`에서는 설치된 CLI가 지원하는 생성 대상과 추가
기능, 전체 옵션을 확인할 수 있습니다.

## `generate`

```text
openapi-sdkgen generate \
  --input <path|file-url|http-url|-> \
  --target <target> \
  --output <directory>
```

### 필수 옵션

- `--input <openapi>`: SDK 생성에 사용할 OpenAPI 3.0.x, 3.1.x, 3.2.x JSON
  또는 YAML 파일. 로컬 경로, `file://` URL, HTTP(S) URL, 표준 입력을 뜻하는
  `-`를 사용할 수 있습니다.
- `--target typescript`: TypeScript SDK를 생성합니다.
- `--output <directory>`: 생성된 코드를 저장할 빈 디렉터리입니다.

`--output`은 항상 디렉터리 경로를 받습니다. `--input -`와 달리
`--output -`는 표준 출력을 뜻하지 않습니다.

## OpenAPI 파일 가져오기

로컬 파일이나 URL을 `--input`에 지정할 수 있습니다.

```sh
# 로컬 파일
openapi-sdkgen generate --input ./openapi.yaml --target typescript --output ./src/generated/api

# file URL
openapi-sdkgen generate --input file:///workspace/openapi.yaml --target typescript --output ./src/generated/api

# 개발 서버
openapi-sdkgen generate --input http://localhost:4010/openapi.json --target typescript --output ./src/generated/api
```

다른 명령이 출력한 OpenAPI 파일을 사용하려면 `--input -`를 지정합니다. `-`는
파일 경로 대신 표준 입력(stdin)을 읽으라는 뜻입니다.

```sh
curl https://api.example.test/openapi.json | \
  openapi-sdkgen generate \
    --input - \
    --target typescript \
    --output ./src/generated/api
```

표준 입력으로 읽은 OpenAPI 파일에 상대 `$ref`가 있다면 기준 경로나 URL을
`--input-base`로 지정하세요.

```sh
curl https://api.example.test/openapi.yaml | \
  openapi-sdkgen generate \
    --input - \
    --input-base https://api.example.test/openapi.yaml \
    --target typescript \
    --output ./src/generated/api
```

## 인증이 필요한 URL

HTTP 요청 헤더의 값은 환경 변수에서 읽을 수 있습니다. 토큰을 명령줄에 직접
넣지 않아도 됩니다.

```sh
export OPENAPI_TOKEN='...'
openapi-sdkgen generate \
  --input https://api.internal.example/openapi.yaml \
  --http-header-env Authorization=OPENAPI_TOKEN \
  --target typescript \
  --output ./src/generated/api
```

`--http-header-env`는 `Header-Name=ENV_VAR` 형식이며 여러 번 사용할 수
있습니다. 빈 값, 잘못된 헤더 이름, 중복 헤더는 거부됩니다. `Host`, `Cookie`,
연결 관리 헤더, 프록시 인증 헤더는 지정할 수 없습니다.

`http://` URL에 인증 헤더를 보내면 암호화되지 않은 연결이라는 경고가
출력됩니다.

### 클라이언트 인증서와 사설 CA

```sh
openapi-sdkgen generate \
  --input https://api.internal.example/openapi.yaml \
  --tls-client-cert ./secrets/openapi-client.pem \
  --tls-client-key ./secrets/openapi-client-key.pem \
  --tls-ca-file ./certs/internal-ca.pem \
  --target typescript \
  --output ./src/generated/api
```

클라이언트 인증서와 키는 함께 지정해야 합니다. `--tls-ca-file`은 시스템 인증서
저장소에 사설 CA를 추가하며 TLS 검증을 끄지 않습니다.

## Webhook과 Callback

```text
--with server
```

OpenAPI에 정의된 Webhook과 Callback을 처리하는 타입과 Fetch 기반 라우터를
`server/` 아래에 생성합니다. 자세한 사용법은
[Webhook과 Callback 처리](../guide/server.md)에서 확인하세요.

## 오류와 경고

`generate`는 OpenAPI 파일을 검사한 뒤 코드를 생성합니다. 경고가 있어도 생성은
계속되지만 오류가 있으면 기존 생성 결과를 바꾸지 않습니다. 오류 메시지에는
가능한 경우 OpenAPI 파일에서 문제가 발생한 위치가 함께 표시됩니다.

별도의 `validate` 명령은 없습니다. CI에서 생성 가능 여부를 확인하는 방법은
[SDK 생성](../guide/generate.md)을 참고하세요.

## 원격 `$ref`

로컬 OpenAPI 파일의 상대 `$ref`는 파일이 있는 디렉터리 안에서만 가져올 수
있습니다. 다른 서버의 `$ref`를 가져오려면 허용할 HTTPS origin을 명시해야
합니다.

- `--allow-remote-ref <origin>`: 원격 `$ref`를 가져올 HTTPS origin을
  허용합니다. 여러 origin은 옵션을 반복해서 지정합니다.
- `--ref-lock <path>`: 참조 잠금 파일의 경로를 지정합니다.
- `--update-ref-lock`: 원격 참조를 가져온 뒤 잠금 파일을 만들거나 갱신합니다.
- `--offline`: 네트워크에 연결하지 않고 이전에 저장한 원격 참조만 사용합니다.

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api \
  --allow-remote-ref https://schemas.example.test \
  --update-ref-lock
```

HTTP(S) URL로 OpenAPI 파일을 읽을 때 같은 origin의 상대 `$ref`는 자동으로
해석합니다. 처음 가져올 때는 잠금 파일을 갱신해야 하며, 다른 origin은
`--allow-remote-ref`로 별도 허용해야 합니다.

인증 헤더, 클라이언트 인증서, 사설 CA는 OpenAPI 파일과 같은 origin에만
사용됩니다. 다른 origin의 `$ref`나 리디렉션에는 전달하지 않습니다.

## JSON Schema 확장 프로그램

```text
--schema-extension <manifest>
```

필수 사용자 정의 JSON Schema vocabulary를 처리할 로컬 확장 프로그램을
등록합니다. 여러 개를 사용하려면 옵션을 반복해서 지정하세요. 확장 프로그램은
SDK 생성 중에만 실행되고 생성된 애플리케이션 코드에는 포함되지 않습니다.
