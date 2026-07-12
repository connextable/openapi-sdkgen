import { createServer } from "node:http";

const port = Number(process.env.TODO_API_PORT ?? "18789");
const todos = [];

const readJSON = async (request) => {
  const chunks = [];
  for await (const chunk of request) chunks.push(Buffer.from(chunk));
  return JSON.parse(Buffer.concat(chunks).toString("utf8"));
};

const server = createServer(async (request, response) => {
  const url = new URL(request.url ?? "/", `http://${request.headers.host}`);
  if (request.method === "POST" && url.pathname === "/v1/todos") {
    const input = await readJSON(request);
    const todo = { id: String(todos.length + 1), title: input.title, completed: false };
    todos.push(todo);
    response.writeHead(201, { "content-type": "application/json" }).end(JSON.stringify(todo));
    return;
  }
  if (request.method === "GET" && url.pathname === "/v1/todos") {
    const completed = url.searchParams.get("completed");
    const items = completed === null ? todos : todos.filter((todo) => String(todo.completed) === completed);
    response.writeHead(200, { "content-type": "application/json" }).end(JSON.stringify({ items }));
    return;
  }
  response.writeHead(404, { "content-type": "application/json" }).end(JSON.stringify({ error: "not found" }));
});

server.listen(port, "127.0.0.1", () => {
  console.log(`Todo API listening at http://127.0.0.1:${port}/v1`);
});
