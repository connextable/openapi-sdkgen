# JavaScript Todo App

Complete JavaScript consumer flow. The OpenAPI document is given to an
externally installed `openapi-sdkgen` CLI, which writes native ESM source into
`src/generated/todo-sdk/`. The app then imports that relative path directly;
there is no SDK package, symlink, TypeScript compiler, or SDK build step.

## Run

Node 22 or newer and `openapi-sdkgen` on `PATH` required.

```sh
./setup.sh
```

Run server and client separately:

```sh
# terminal 1
node src/server.js

# terminal 2
TODO_API_BASE_URL=http://127.0.0.1:18789/v1 node src/client.js
```

## Manual flow

```sh
openapi-sdkgen generate \
  --input openapi.json \
  --target javascript \
  --output src/generated/todo-sdk

# terminal 1
TODO_API_PORT=18789 node src/server.js

# terminal 2
TODO_API_BASE_URL=http://127.0.0.1:18789/v1 node src/client.js
```

The JavaScript target intentionally exposes calls by exact `operationId`:

```js
import { createClient } from "./generated/todo-sdk/index.js";

const api = createClient({ baseURL });
const created = await api.$operations.createTodo({ body: { title: "Write docs" } });
const listed = await api.$operations.listTodos({ query: { completed: false } });
```
