# CLI 레퍼런스

## Command

```text
openapi-sdkgen generate --input <document> --target typescript --output <directory>
  [--with <addon> ...]
  [--allow-remote-ref <https-origin> ...]
  [--ref-lock <path>]
  [--update-ref-lock]
  [--offline]
  [--schema-extension <manifest> ...]
```

## 필수 option

- `--input <document>` — 생성할 OpenAPI 3.0.x, 3.1.x, 3.2.x JSON 문서입니다.
- `--target typescript` — 현재 지원하는 source-mode target입니다.
- `--output <directory>` — 생성 artifact를 둘 비어 있는 애플리케이션 소스 디렉터리입니다.

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
거부됩니다. 정확한 origin을 지정하기 전에는 network resolution이 비활성화되어 있습니다.

## Schema extension

- `--schema-extension <manifest>` — 필수 custom JSON Schema vocabulary용 trusted local compiler
  extension을 등록합니다. 여러 manifest를 등록하려면 이 option을 반복해서 지정하세요.

extension manifest는 executable digest와 vocabulary URI를 lock합니다. extension protocol은 compile-time
JSON-RPC만 사용하며, executable TypeScript나 runtime callback 대신 replacement JSON Schema object 또는
boolean을 반환합니다.
