# CLI 레퍼런스

## Command

```text
openapi-sdkgen generate --input <path|file-url|http-url|-> --target typescript --output <directory>
  [--input-base <document>]
  [--http-header-env <Header-Name=ENV_VAR> ...]
  [--tls-client-cert <path> --tls-client-key <path>]
  [--tls-ca-file <path>]
  [--with <addon> ...]
  [--allow-remote-ref <https-origin> ...]
  [--ref-lock <path>]
  [--update-ref-lock]
  [--offline]
  [--schema-extension <manifest> ...]
```

## 비공개 HTTP(S) 입력

명령줄에 값을 넣지 않고 요청 헤더를 보내려면 환경 변수 매핑을 사용합니다.
`--http-header-env`는 여러 번 지정할 수 있습니다. 매핑 형식은 정확히
`Header-Name=ENV_VAR`입니다. 헤더 이름은 HTTP token 문법을, 환경 변수 이름은
`[A-Za-z_][A-Za-z0-9_]*`를 따라야 합니다. 값은 비어 있으면 안 되며, 대소문자를
구분하지 않는 중복 헤더 이름은 거부됩니다.

```sh
export OPENAPI_TOKEN='...'
openapi-sdkgen generate \
  --input https://api.internal.example/openapi.yaml \
  --http-header-env Authorization=OPENAPI_TOKEN \
  --target typescript \
  --output ./src/generated/api
```

`Host`, `Cookie`, connection 관리 헤더, transfer 헤더, proxy authorization 헤더는
지정할 수 없습니다. 설정한 헤더는 sdkgen 기본 헤더 뒤에 적용되므로 `Accept`를
지정하면 기본 `Accept` 값을 대체합니다. 헤더 매핑은 HTTP(S) 루트 입력에서만 쓸 수
있습니다. `http://` 입력에 매핑 헤더를 보내면 연결에서 기밀성이 보장되지 않으므로
sdkgen이 한 번 경고를 출력합니다.

비공개 TLS에는 인증서/키 쌍과 PEM CA bundle을 지정할 수 있습니다.

```sh
openapi-sdkgen generate \
  --input https://api.internal.example/openapi.yaml \
  --tls-client-cert ./secrets/openapi-client.pem \
  --tls-client-key ./secrets/openapi-client-key.pem \
  --tls-ca-file ./certs/internal-ca.pem \
  --target typescript \
  --output ./src/generated/api
```

클라이언트 인증서와 키는 함께 지정해야 합니다. CA 파일에는 유효한 PEM
`CERTIFICATE` block만 들어갈 수 있으며 system trust store에 추가됩니다. 이 옵션은
TLS 검증을 끄지 않습니다.

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

헤더 매핑과 비공개 TLS 설정은 루트와 정확히 같은 origin에만 적용됩니다. scheme, host,
명시한 port가 모두 같아야 합니다. same-origin `$ref`는 이 설정을 상속하지만, 다른 origin으로
나가는 redirect는 거부합니다. allowlist로 허용한 cross-origin `$ref`에는 해당 헤더, client
certificate, 추가 CA root를 전달하지 않습니다. client certificate 또는 추가 CA root를 쓰는
경우 standard proxy 환경 변수가 선택한 `https://` proxy는 연결하기 전에 거부합니다. 일반 HTTP,
SOCKS proxy와 `NO_PROXY` 동작은 그대로 사용할 수 있습니다.

보호된 same-origin `$ref` 본문도 일반 reference lock과 cache를 사용합니다. 다만 sdkgen은 cache
directory 권한을 `0700`, entry 권한을 `0600`으로 줄입니다. cache root와 entry는 symlink가 아닌
directory/regular file이어야 하며, 안전하지 않은 경로는 online/offline 모두에서 실패합니다. local
보관 정책이 맞지 않으면 cache를 삭제하세요. Windows는 이 owner-only mode 계약을 강제할 수 없으므로
보호된 remote reference cache를 저장하기 전에 실패합니다.
다른 운영체제에서도 filesystem이 hard link를 지원하지 않으면 보호된 cache digest를 게시하기 전에
실패합니다. 비보호 cache는 기존 rename fallback을 유지합니다.

이번 범위에는 OAuth/SSO browser flow, cloud request signing, credential store, custom fetch command,
cross-origin credential 공유가 포함되지 않습니다.

## Schema extension

- `--schema-extension <manifest>` — 필수 custom JSON Schema vocabulary용 trusted local compiler
  extension을 등록합니다. 여러 manifest를 등록하려면 이 option을 반복해서 지정하세요.

extension manifest는 executable digest와 vocabulary URI를 lock합니다. extension protocol은 compile-time
JSON-RPC만 사용하며, executable TypeScript나 runtime callback 대신 replacement JSON Schema object 또는
boolean을 반환합니다.
