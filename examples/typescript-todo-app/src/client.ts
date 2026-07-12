import { createClient } from "./generated/todo-sdk/index.js";

const baseURL = process.env.TODO_API_BASE_URL ?? "http://127.0.0.1:18787/v1";
const api = createClient({ baseURL });

const title = "Generate and use this SDK";
const created = await api.todos.create({
  body: { title },
});
const listed = await api.todos.list({ query: { completed: false } });

// The local server starts empty, so this proves request encoding and response decoding.
if (created.id !== "1" || created.title !== title || created.completed) {
  throw new Error(`unexpected created todo: ${JSON.stringify(created)}`);
}

// This proves the query parameter reached the server and the filtered result decoded.
if (listed.items.length !== 1 || listed.items[0]?.id !== created.id) {
  throw new Error(`unexpected filtered todos: ${JSON.stringify(listed)}`);
}

console.log(JSON.stringify({ created, listed }, null, 2));
