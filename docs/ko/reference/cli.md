# CLI 레퍼런스

## Command

```text
openapi-sdkgen generate --input <path|file-url|http-url|-> --target typescript --output <directory>
  [--input-base <document>]
  [--with <addon> ...]
  [--allow-remote-ref <https-origin> ...]
  [--ref-lock <path>]
  [--update-ref-lock]
  [--offline]
  [--schema-extension <manifest> ...]
```

## 필수 option

- `--input <document>` — 생성할 OpenAPI 3.0.x, 3.1.x, 3.2.x JSON 또는 YAML 문서입니다.
  local path, `file://` URL, HTTP(S) URL, stdin을 뜻하는 `-`를 사용할 수 있습니다.
- `--target typescript` — 현재 지원하는 source-mode target입니다.
- `--output <directory>` — 생성 artifact를 둘 비어 있는 애플리케이션 소스 디렉터리입니다.

## 입력 source

`--input`은 루트 문서를 지정합니다. `$ref`가 아니므로 HTTP(S) 입력을 읽는 데
`--allow-remote-ref`가 필요하지 않고 reference lock에도 기록하지 않습니다. loopback과 private
development endpoint도 루트 입력으로 사용할 수 있습니다.

```sh
# local file 또는 file URL
openapi-sdkgen generate --input ./openapi.yaml --target typescript --output ./src/generated/api
openapi-sdkgen generate --input file:///workspace/openapi.yaml --target typescript --output ./src/generated/api

# HTTP(S) endpoint
openapi-sdkgen generate --input http://localhost:4010/openapi.json --target typescript --output ./src/generated/api

# 문서 바이트를 출력할 수 있는 모든 producer
curl https://api.example.test/openapi.json | \
  openapi-sdkgen generate --input - --target typescript --output ./src/generated/api
```

stdin에는 상대 `$ref`의 기준 위치가 없습니다. 필요한 경우에만 `--input-base`로 원본 문서 위치를
지정하세요.

```sh
curl https://api.example.test/openapi.yaml | \
  openapi-sdkgen generate \
    --input - \
    --input-base https://api.example.test/openapi.yaml \
    --target typescript \
    --output ./src/generated/api \
    --ref-lock ./openapi.refs.lock \
    --update-ref-lock
```

## Optional add-on

- `--with server` — `server/`에 Fetch-native Callback과 Webhook 계약을 추가합니다.
  미래 add-on을 결합할 수 있도록 `--with`는 반복해서 사용할 수 있습니다.

## 원격 reference 정책

- `--allow-remote-ref <origin>` — 원격 `$ref` 해석을 위해 정확한 HTTPS origin 하나를 허용합니다.
  여러 origin을 허용하려면 이 option을 반복해서 지정하세요.
- `--ref-lock <path>` — remote reference와 extension integrity lock 경로를 override합니다.
- `--update-ref-lock` — 성공한 compile 후에만 lock을 생성하거나 갱신합니다.
- `--offline` — local content-addressed cache에서만 잠긴 remote reference를 해석합니다.

local file reference는 input directory 안에 유지되어야 합니다. canonical root 밖을 가리키는 reference는
거부됩니다. 정확한 origin을 지정하기 전에는 cross-origin network resolution이 비활성화되어 있습니다.

HTTP(S) 루트 문서의 same-origin 상대 `$ref`는 루트 URL을 기준으로 해석합니다. 이들은 여전히 remote
reference이므로 처음에는 `--ref-lock`과 `--update-ref-lock`이 필요합니다. 다른 origin의 `$ref`는 계속
`--allow-remote-ref`를 명시해야 합니다. `--offline`은 network connection을 열지 않으므로 HTTP(S) 루트
입력과 함께 사용할 수 없습니다. local file 또는 stdin을 사용하세요.

## Schema extension

- `--schema-extension <manifest>` — 필수 custom JSON Schema vocabulary용 trusted local compiler
  extension을 등록합니다. 여러 manifest를 등록하려면 이 option을 반복해서 지정하세요.

extension manifest는 executable digest와 vocabulary URI를 lock합니다. extension protocol은 compile-time
JSON-RPC만 사용하며, executable TypeScript나 runtime callback 대신 replacement JSON Schema object 또는
boolean을 반환합니다.
