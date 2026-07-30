# Getting started

`openapi-sdkgen` is a CLI that generates application SDK source from an OpenAPI
3.x file. This guide uses an OpenAPI JSON or YAML file to generate a TypeScript
client and make the first API call.

Use `--input` to select the OpenAPI JSON or YAML file and `--output` to select
an empty directory for the generated code.

## 1. Install the CLI

Use the precompiled npm CLI in a normal Node-based application project. It
contains the platform executable, so consumers do not need Go:

```sh
pnpm dlx openapi-sdkgen generate \
  --input ./openapi.yaml \
  --target typescript \
  --output ./src/generated/api
```

You can also use the GitHub Release binary directly. Go users can install the
same CLI from the module:

```sh
go install github.com/connextable/openapi-sdkgen/cmd/openapi-sdkgen@latest
```

On macOS or Linux, Homebrew installs the CLI without a separate Go setup:

```sh
brew install connextable/tap/openapi-sdkgen
```

## 2. Generate into application source

```sh
openapi-sdkgen generate \
  --input ./openapi.json \
  --target typescript \
  --output ./src/generated/api
```

The output directory must be empty.

The root document can also come from a `file://` URL, an HTTP(S) development
server, or stdin. It is a source, not a remote `$ref`:

```sh
openapi-sdkgen generate \
  --input http://localhost:4010/openapi.json \
  --target typescript \
  --output ./src/generated/api
```

Use `--input -` when another command supplies the OpenAPI file. The `-` means
standard input (stdin) instead of a file path. If the OpenAPI file uses relative
`$ref` values, add `--input-base <path-or-url>`.

## 3. Import the client in your web application

```ts
import { createClient } from "./generated/api";

const api = createClient({
  baseURL: "https://api.example.test/v1",
});
```

Vite, Next.js, Nuxt, and similar web bundlers resolve the generated directory
to its `index.ts` entry.

::: details Direct Node ESM

Node ESM does not resolve relative directories as `index.js`. If the
application compiles and executes directly in Node, import
`./generated/api/index.js` instead.
:::

## 4. Call generated resources

```ts
const todo = await api.todos.create({
  body: { title: "Write documentation" },
});

const page = await api.todos.list({
  query: { limit: 20 },
});
```

Resource methods provide the most readable API for application code. Every API
can also be called by HTTP method and OpenAPI path through `api.$routes`.
When an `operationId` is declared, `api.$operations` is also available.

Next: [generate an SDK](./generate.md) or [use the client](./client.md).
