# Getting started

`openapi-sdkgen` turns a valid OpenAPI 3.0.x, 3.1.x, or 3.2.x document into
ordinary TypeScript source. The generated files are part of the application;
they are not a separately built npm package.

The input is an OpenAPI [OpenAPI Object](https://spec.openapis.org/oas/v3.2.0.html#openapi-object)
whose [Paths Object](https://spec.openapis.org/oas/v3.2.0.html#paths-object) and
[Operation Object](https://spec.openapis.org/oas/v3.2.0.html#operation-object)
describe the calls the client will expose.

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

The output directory must be fresh. Generation writes all artifacts to a
staging area and publishes them only after every artifact succeeds.

The root document can also come from a `file://` URL, an HTTP(S) development
server, or stdin. It is a source, not a remote `$ref`:

```sh
openapi-sdkgen generate \
  --input http://localhost:4010/openapi.json \
  --target typescript \
  --output ./src/generated/api
```

Use `--input -` when another command supplies the document. If that document
uses relative `$ref` values, add `--input-base <path-or-url>`.

## 3. Import the client in your web application

```ts
import { createClient } from "./generated/api";

const api = createClient({
  baseURL: "https://api.example.test/v1",
});
```

Vite, Next.js, Nuxt, and similar web bundlers resolve the generated directory
to its `index.ts` entry. No generated package manifest or separate SDK build
step is required.

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

Named resources offer the ergonomic API. Every operation also remains available
by its exact `operationId` through `api.$operations`.

Next: [generate an SDK](./generate.md) or [use the client](./client.md).
