# CLI reference

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

## Required options

- `--input <document>` — An OpenAPI 3.0.x, 3.1.x, or 3.2.x JSON or YAML
  document. Pass a local path, `file://` URL, HTTP(S) URL, or `-` for stdin.
- `--target typescript` — The active source-mode target.
- `--output <directory>` — A fresh application-source directory for generated artifacts.

## Input sources

`--input` names the root document. It is not a `$ref`, so reading an HTTP(S)
input does not need `--allow-remote-ref` or create a reference-lock entry.
Loopback and private development endpoints are valid root inputs.

```sh
# Local file or file URL
openapi-sdkgen generate --input ./openapi.yaml --target typescript --output ./src/generated/api
openapi-sdkgen generate --input file:///workspace/openapi.yaml --target typescript --output ./src/generated/api

# HTTP(S) endpoint
openapi-sdkgen generate --input http://localhost:4010/openapi.json --target typescript --output ./src/generated/api

# Any producer that can write document bytes
curl https://api.example.test/openapi.json | \
  openapi-sdkgen generate --input - --target typescript --output ./src/generated/api
```

Stdin has no location for relative `$ref` values. Supply the source document
location through `--input-base` only when stdin input needs one:

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

## Optional add-ons

- `--with server` — Adds Fetch-native Callback and Webhook contracts under
  `server/`. Repeat `--with` when combining future add-ons.

## Remote reference policy

- `--allow-remote-ref <origin>` — Permits one exact HTTPS origin for remote
  `$ref` resolution. Repeat the option to permit more than one origin.
- `--ref-lock <path>` — Overrides the remote-reference and extension integrity
  lock path.
- `--update-ref-lock` — Creates or updates the lock only after a successful
  compile.
- `--offline` — Resolves locked remote references only from the local
  content-addressed cache.

Local file references remain contained within the input directory. A reference
outside that canonical root is rejected. Cross-origin remote resolution is off
until an exact origin is supplied.

For an HTTP(S) root document, same-origin relative `$ref` values resolve from
the root URL. They remain remote references, so use `--ref-lock` and
`--update-ref-lock` on the first run. A `$ref` at another origin still needs
`--allow-remote-ref`. `--offline` never opens a network connection and rejects
an HTTP(S) root input; provide a local file or stdin instead.

## Schema extensions

- `--schema-extension <manifest>` — Registers a trusted local compiler
  extension for a required custom JSON Schema vocabulary. Repeat the option
  when registering multiple manifests.

An extension manifest locks its executable digest and vocabulary URIs. The
extension protocol is compile-time JSON-RPC only: it returns a replacement JSON
Schema object or boolean, never executable TypeScript or runtime callbacks.
