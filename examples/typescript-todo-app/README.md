# TypeScript Todo App

This is a complete consumer example: generate an SDK from `openapi.json`, install it into a normal TypeScript app as `file:./sdk`, then call it over real HTTP. `src/server.ts` is a tiny local Todo API; `src/client.ts` is a separate SDK consumer process. No credentials or external network service required.

## Run

Install the `openapi-sdkgen` CLI, Node 22 or newer, and pnpm 10 or newer. The example treats `openapi-sdkgen` as an externally installed command on `PATH`, exactly as an application repository would.

Prepare the SDK and app:

```sh
./setup.sh
```

`setup.sh` deliberately generates a fresh `sdk/` directory, installs and builds that SDK package, then installs and builds the app. No test fixture or symlink is involved.

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
  --output sdk \
  --package-name @example/todo-sdk
pnpm --dir sdk install
pnpm --dir sdk run build
pnpm install
pnpm run build

# terminal 1
TODO_API_PORT=18787 pnpm run server

# terminal 2
TODO_API_BASE_URL=http://127.0.0.1:18787/v1 pnpm run client
```

`src/client.ts` imports the generated SDK just as an app would:

```ts
import { createClient } from "@example/todo-sdk";

const api = createClient({ baseURL });
const created = await api.todos.create({ body: { title: "Write docs" } });
const listed = await api.todos.list({ query: { completed: false } });
```
