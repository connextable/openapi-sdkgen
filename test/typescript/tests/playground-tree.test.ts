import { describe, expect, it } from "vitest";

import { buildArtifactTree, findArtifact } from "../../../docs/.vitepress/theme/playground/tree.js";

const artifacts = [
  { path: "z-last.ts", content: "export const last = true" },
  { path: "internal/routes/index.ts", content: "export interface Routes {}" },
  {
    path: "internal/operations/users/by-user-id/get.ts",
    content: "export type RouteKey = 'GET /users/{userId}'",
  },
  { path: "internal/client/index.ts", content: "export { createClient }" },
  { path: "a-first.ts", content: "export const first = true" },
] as const;

describe("playground artifact tree", () => {
  it("sorts directories before files at every level", () => {
    const tree = buildArtifactTree(artifacts);

    expect(tree.map((node) => `${node.type}:${node.name}`)).toEqual([
      "directory:internal",
      "file:a-first.ts",
      "file:z-last.ts",
    ]);
    expect(tree[0]?.children?.map((node) => node.name)).toEqual(["client", "operations", "routes"]);
  });

  it("retains paths through at least three nested directory levels", () => {
    const tree = buildArtifactTree(artifacts);
    const internal = tree.find((node) => node.path === "internal");
    const operations = internal?.children?.find((node) => node.path === "internal/operations");
    const users = operations?.children?.find((node) => node.path === "internal/operations/users");
    const byUserID = users?.children?.find(
      (node) => node.path === "internal/operations/users/by-user-id",
    );

    expect(byUserID?.children).toEqual([
      {
        name: "get.ts",
        path: "internal/operations/users/by-user-id/get.ts",
        type: "file",
      },
    ]);
  });

  it("resolves the selected path to the exact generated content", () => {
    expect(findArtifact(artifacts, "internal/routes/index.ts")?.content).toBe(
      "export interface Routes {}",
    );
    expect(findArtifact(artifacts, "internal/routes/missing.ts")).toBeUndefined();
  });
});
