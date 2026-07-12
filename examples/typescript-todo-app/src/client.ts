import { createClient } from "@example/todo-sdk";

const baseURL = process.env.TODO_API_BASE_URL ?? "http://127.0.0.1:18787/v1";
const api = createClient({ baseURL });

const created = await api.todos.create({ body: { title: "Generate and use this SDK" } });
const listed = await api.todos.list({ query: { completed: false } });

console.log(JSON.stringify({ created, listed }, null, 2));
