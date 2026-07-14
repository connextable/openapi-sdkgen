# CLI reference

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

## Private HTTP(S) inputs

Use environment-variable mappings to send a request header without placing its
value in the command line. Repeat `--http-header-env` for multiple headers.
Each mapping is exactly `Header-Name=ENV_VAR`: header names use HTTP token
syntax, environment variable names use `[A-Za-z_][A-Za-z0-9_]*`, values must be
non-empty, and duplicate names are rejected case-insensitively.

```sh
export OPENAPI_TOKEN='...'
openapi-sdkgen generate \
  --input https://api.internal.example/openapi.yaml \
  --http-header-env Authorization=OPENAPI_TOKEN \
  --target typescript \
  --output ./src/generated/api
```

`Host`, `Cookie`, connection-management headers, transfer headers, and proxy
authorization headers are rejected. The configured headers are applied after
sdkgen's defaults, so a configured `Accept` header replaces the default
`Accept` value. Header mappings are valid only for an HTTP(S) root input. If a
mapped header is sent to an `http://` input, sdkgen prints one warning because
the header is not confidential on that connection.

For private TLS, pass a certificate/key pair and/or a PEM CA bundle:

```sh
openapi-sdkgen generate \
  --input https://api.internal.example/openapi.yaml \
  --tls-client-cert ./secrets/openapi-client.pem \
  --tls-client-key ./secrets/openapi-client-key.pem \
  --tls-ca-file ./certs/internal-ca.pem \
  --target typescript \
  --output ./src/generated/api
```

The client certificate and key must be provided together. The CA file must
contain only valid PEM `CERTIFICATE` blocks; it augments the system trust store.
These options do not disable TLS verification.

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

Header mappings and private TLS settings apply only to the exact root origin:
scheme, host, and explicit port must all match. Same-origin `$ref` requests
inherit them; redirects that would leave that origin are rejected. Allowlisted
cross-origin `$ref` requests never receive those headers, client certificates,
or added CA roots. With a client certificate or added CA root, sdkgen rejects
an `https://` proxy selected by the standard proxy environment variables before
dialing it; ordinary HTTP and SOCKS proxy behavior, including `NO_PROXY`, stays
available.

Protected same-origin `$ref` bodies use the normal reference lock and cache,
but sdkgen narrows the cache directory to `0700` and entries to `0600`. Cache
roots and entries must be non-symlink directory/regular-file paths; unsafe
paths fail closed both online and offline. Remove the cache if its local
retention policy is not acceptable. Windows cannot enforce this owner-only
mode contract, so protected remote-reference caching fails before persistence
on Windows. On other filesystems without hard-link support, protected caching
also fails before digest publication; unprotected caching retains its rename
fallback.

This phase does not implement OAuth/SSO browser flows, cloud-request signing,
credential stores, custom fetch commands, or cross-origin credential sharing.

## Schema extensions

- `--schema-extension <manifest>` — Registers a trusted local compiler
  extension for a required custom JSON Schema vocabulary. Repeat the option
  when registering multiple manifests.

An extension manifest locks its executable digest and vocabulary URIs. The
extension protocol is compile-time JSON-RPC only: it returns a replacement JSON
Schema object or boolean, never executable TypeScript or runtime callbacks.
