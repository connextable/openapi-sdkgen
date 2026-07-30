# CLI reference

## Help and version

```sh
openapi-sdkgen --help
openapi-sdkgen generate --help
openapi-sdkgen --version
```

`openapi-sdkgen --help` lists commands.
`openapi-sdkgen generate --help` lists every generation target, optional
feature, and flag supported by the installed CLI.

## `generate`

```text
openapi-sdkgen generate \
  --input <path|file-url|http-url|-> \
  --target <target> \
  --output <directory>
```

### Required options

- `--input <openapi>`: an OpenAPI 3.0.x, 3.1.x, or 3.2.x JSON or YAML file.
  Use a local path, `file://` URL, HTTP(S) URL, or `-` for stdin.
- `--target typescript`: generates a TypeScript SDK.
- `--output <directory>`: an empty directory for generated code.

`--output` always expects a directory path. Unlike `--input -`, `--output -`
does not mean standard output.

## Read an OpenAPI file

Pass a local file or URL to `--input`.

```sh
# Local file
openapi-sdkgen generate --input ./openapi.yaml --target typescript --output ./src/generated/api

# file URL
openapi-sdkgen generate --input file:///workspace/openapi.yaml --target typescript --output ./src/generated/api

# Development server
openapi-sdkgen generate --input http://localhost:4010/openapi.json --target typescript --output ./src/generated/api
```

Use `--input -` to read an OpenAPI file from another command. The `-` means
standard input (stdin) instead of a file path.

```sh
curl https://api.example.test/openapi.json | \
  openapi-sdkgen generate \
    --input - \
    --target typescript \
    --output ./src/generated/api
```

When the OpenAPI file uses relative `$ref` values, use `--input-base` to set
the base path or URL.

```sh
curl https://api.example.test/openapi.yaml | \
  openapi-sdkgen generate \
    --input - \
    --input-base https://api.example.test/openapi.yaml \
    --target typescript \
    --output ./src/generated/api
```

## Authenticated URLs

Read HTTP request header values from environment variables so secrets do not
appear in the command line.

```sh
export OPENAPI_TOKEN='...'
openapi-sdkgen generate \
  --input https://api.internal.example/openapi.yaml \
  --http-header-env Authorization=OPENAPI_TOKEN \
  --target typescript \
  --output ./src/generated/api
```

`--http-header-env` uses the format `Header-Name=ENV_VAR` and may be repeated.
Empty values, invalid header names, and duplicate headers are rejected. `Host`,
`Cookie`, connection-management headers, and proxy authorization headers
cannot be set.

The CLI warns when a mapped header is sent over an unencrypted `http://`
connection.

### Client certificates and private CAs

```sh
openapi-sdkgen generate \
  --input https://api.internal.example/openapi.yaml \
  --tls-client-cert ./secrets/openapi-client.pem \
  --tls-client-key ./secrets/openapi-client-key.pem \
  --tls-ca-file ./certs/internal-ca.pem \
  --target typescript \
  --output ./src/generated/api
```

The client certificate and key must be provided together. `--tls-ca-file` adds
a private CA to the system trust store; it does not disable TLS verification.

## Webhooks and Callbacks

```text
--with server
```

Generate handler types and Fetch-based routers under `server/`. See
[Handle Webhooks and Callbacks](../guide/server.md) for examples.

## Errors and warnings

`generate` checks the OpenAPI file before writing code. Warnings do not stop
generation. Errors leave existing generated code unchanged and identify the
relevant OpenAPI location when possible.

There is no separate `validate` command. See
[Generate an SDK](../guide/generate.md) for CI use.

## Remote `$ref` values

Relative file references must stay within the OpenAPI file's directory tree.
To fetch a `$ref` from another server, allow its exact HTTPS origin.

- `--allow-remote-ref <origin>`: allow one HTTPS origin. Repeat the option for
  more than one origin.
- `--ref-lock <path>`: set the reference lock file path.
- `--update-ref-lock`: fetch remote references and create or update the lock.
- `--offline`: use previously cached references without network access.

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api \
  --allow-remote-ref https://schemas.example.test \
  --update-ref-lock
```

For an HTTP(S) OpenAPI URL, same-origin relative `$ref` values are resolved
automatically. Update the lock the first time they are fetched. A different
origin still requires `--allow-remote-ref`.

Authentication headers, client certificates, and private CAs are only used for
the same origin as the OpenAPI file. They are never forwarded to a different
origin or redirect.

## JSON Schema extensions

```text
--schema-extension <manifest>
```

Register a local extension for a required custom JSON Schema vocabulary.
Repeat the option to register more than one extension. Extensions run only
while generating the SDK and are not included in application code.
