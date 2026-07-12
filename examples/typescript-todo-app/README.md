# TypeScript Todo App

This is a complete consumer example: generate TypeScript source from `openapi.json` into the app's `src/generated/todo-sdk/` directory, then call it over real HTTP. `src/server.ts` is a tiny local Todo API; `src/client.ts` is a separate SDK consumer process. No credentials or external network service required.

## Run

Install the `openapi-sdkgen` CLI, Node 22 or newer, and pnpm 10 or newer. The example treats `openapi-sdkgen` as an externally installed command on `PATH`, exactly as an application repository would.

Generate the client source and build the app:

```sh
./setup.sh
```

`setup.sh` deliberately generates a fresh `src/generated/todo-sdk/` directory, then installs and builds the app. The generated source is compiled by the application's normal TypeScript build; no SDK package, symlink, or second build is involved.

Run the separate processes in two terminals:

```sh
# terminal 1
pnpm run server

# terminal 2
TODO_API_BASE_URL=http://127.0.0.1:18787/v1 pnpm run client
```

## Manual flow

```sh
openapi-sdkgen generate \
  --input openapi.json \
  --target typescript \
  --output src/generated/todo-sdk
pnpm install
pnpm run build

# terminal 1
TODO_API_PORT=18787 pnpm run server

# terminal 2
TODO_API_BASE_URL=http://127.0.0.1:18787/v1 pnpm run client
```

`src/client.ts` imports the generated source through a relative path. The `.js` suffix is intentional: it lets the app use TypeScript's `NodeNext` module resolution and produces valid ESM after compilation.

```ts
import { createClient } from "./generated/todo-sdk/index.js";

const api = createClient({ baseURL });
const created = await api.todos.create({ body: { title: "Write docs" } });
const listed = await api.todos.list({ query: { completed: false } });
```
