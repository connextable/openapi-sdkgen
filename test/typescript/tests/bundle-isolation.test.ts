import { gzipSync } from "node:zlib";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

import { beforeAll, describe, expect, it } from "vitest";
import { build } from "vite";

const fixtureRoot = fileURLToPath(
  new URL("../fixtures/generated/bundle-isolation/", import.meta.url),
);
const publicEntry = join(fixtureRoot, "index.ts");
const metadataEntry = join(fixtureRoot, "metadata.ts");
const internalEntry = (module: string) => join(fixtureRoot, "internal", `${module}.ts`);

type BundleResult = {
  readonly code: string;
  readonly gzipBytes: number;
  readonly modules: readonly string[];
};

const bundleCases = new Map<string, Promise<BundleResult>>();

function bundle(name: string, source: string): Promise<BundleResult> {
  const existing = bundleCases.get(name);
  if (existing !== undefined) return existing;

  const pending = build({
    configFile: false,
    logLevel: "silent",
    plugins: [
      {
        name: "sdkgen-bundle-entry",
        resolveId(id) {
          if (id === "virtual:sdkgen-bundle-entry") return "\0virtual:sdkgen-bundle-entry.ts";
          if (id === "sdkgen-fixture:index") return publicEntry;
          if (id === "sdkgen-fixture:metadata") return metadataEntry;
          if (id.startsWith("sdkgen-fixture:"))
            return internalEntry(id.slice("sdkgen-fixture:".length));
          return null;
        },
        load(id) {
          return id === "\0virtual:sdkgen-bundle-entry.ts" ? source : null;
        },
      },
    ],
    build: {
      target: "es2022",
      minify: "oxc",
      write: false,
      rollupOptions: {
        input: "virtual:sdkgen-bundle-entry",
        preserveEntrySignatures: "strict",
        output: { format: "es", codeSplitting: false },
      },
    },
  }).then(async (result) => {
    if (!Array.isArray(result) && !("output" in result)) {
      await result.close();
      throw new Error("bundle build unexpectedly entered watch mode");
    }
    const outputs = Array.isArray(result) ? result : [result];
    const chunks = outputs
      .flatMap((output) => output.output)
      .filter((item) => item.type === "chunk");
    if (chunks.length !== 1) throw new Error(`bundle emitted ${chunks.length} chunks, want 1`);
    const chunk = chunks[0];
    if (chunk === undefined) throw new Error("bundle emitted no JavaScript chunk");
    return {
      code: chunk.code,
      gzipBytes: gzipSync(chunk.code, { level: 9 }).byteLength,
      modules: chunk.moduleIds,
    };
  });
  bundleCases.set(name, pending);
  return pending;
}

function rootValue(name: string): string {
  return `export { ${name} } from "sdkgen-fixture:index"`;
}

function directValue(name: string, module: string): string {
  return `export { ${name} } from "sdkgen-fixture:${module}"`;
}

function internalModules(result: BundleResult): string[] {
  return result.modules
    .filter((id) => id.startsWith(fixtureRoot))
    .map((id) => id.slice(fixtureRoot.length))
    .filter((id) => id.startsWith("internal/"))
    .sort();
}

function bundleEvidence(result: BundleResult): string {
  return `${result.modules.join("\n")}\nCODE\n${result.code}`;
}

const results: Record<string, BundleResult> = {};

beforeAll(async () => {
  const cases = {
    rootError: rootValue("isAPIError"),
    directError: directValue("isAPIError", "runtime/errors"),
    rootClient: rootValue("createClient"),
    directClient: directValue("createClient", "client/index"),
    rootSort: rootValue("SortDirection"),
    directSort: directValue("SortDirection", "runtime/constants"),
    rootEnums: rootValue("Enums"),
    directEnums: directValue("Enums", "enums"),
    rootType: `import type { Client } from "sdkgen-fixture:index"; export type BundledClient = Client`,
    metadata: `export { openapi } from "sdkgen-fixture:metadata"`,
  };
  await Promise.all(
    Object.entries(cases).map(async ([name, source]) => {
      results[name] = await bundle(name, source);
    }),
  );
});

describe("generated public entry bundle isolation", () => {
  it.each([
    ["rootError", "directError"],
    ["rootClient", "directClient"],
    ["rootSort", "directSort"],
    ["rootEnums", "directEnums"],
  ] as const)("keeps %s within the owning-module gzip budget", (rootName, directName) => {
    expect(results[rootName]?.gzipBytes).toBeLessThanOrEqual(
      (results[directName]?.gzipBytes ?? 0) + 256,
    );
  });

  it("keeps the runtime error guard independent", () => {
    const result = results.rootError;
    expect(result).toBeDefined();
    expect(internalModules(result!), bundleEvidence(result!)).toEqual([
      "internal/runtime/errors.ts",
    ]);
    expect(result!.code).not.toContain("bundle-enum-sentinel-01");
    expect(result!.code).not.toContain("bundle-error-category-sentinel");
    expect(result!.code).not.toContain("bundle-isolation-sentinel");
  });

  it("keeps the client independent from public enum and error-category runtime", () => {
    const result = results.rootClient;
    expect(result).toBeDefined();
    expect(internalModules(result!), bundleEvidence(result!)).toEqual([
      "internal/client/factory.ts",
      "internal/client/registry.ts",
      "internal/operations/bundle-isolation-sentinel/get.ts",
      "internal/resources/bundle-isolation-sentinel/index.ts",
      "internal/resources/root.ts",
      "internal/runtime/callables.ts",
      "internal/runtime/codecs.ts",
      "internal/runtime/errors.ts",
      "internal/runtime/http.ts",
      "internal/runtime/objects.ts",
      "internal/runtime/operation.ts",
      "internal/schemas/isolation-mode.ts",
      "internal/schemas/isolation-record.ts",
      "internal/schemas/isolation-rejected-error.ts",
      "internal/schemas/wire.ts",
    ]);
    expect(result!.code).not.toContain("Symbol.iterator");
    expect(result!.code).not.toContain("bundle-error-category-sentinel");
  });

  it("keeps the sort constant independent", () => {
    const result = results.rootSort;
    expect(result).toBeDefined();
    expect(internalModules(result!), bundleEvidence(result!)).toEqual([
      "internal/runtime/constants.ts",
    ]);
    expect(result!.code).not.toContain("bundle-enum-sentinel-01");
    expect(result!.code).not.toContain("bundle-error-category-sentinel");
    expect(result!.code).not.toContain("bundle-isolation-sentinel");
  });

  it("keeps public enum runtime behind its owning concern", () => {
    const result = results.rootEnums;
    expect(result).toBeDefined();
    expect(internalModules(result!), bundleEvidence(result!)).toEqual(["internal/enums.ts"]);
    expect(result!.code).toContain("bundle-enum-sentinel-01");
    expect(result!.code).not.toContain("bundle-error-category-sentinel");
    expect(result!.code).not.toContain("bundle-isolation-sentinel");
  });

  it("emits no runtime for a type-only root import", () => {
    const result = results.rootType;
    expect(result).toBeDefined();
    expect(internalModules(result!)).toEqual([]);
    expect(result!.code.trim()).toBe("");
  });

  it("keeps metadata at its explicit public subpath", () => {
    const result = results.metadata;
    expect(result).toBeDefined();
    expect(internalModules(result!)).toEqual([]);
    expect(result!.modules.filter((id) => id.startsWith(fixtureRoot))).toEqual([metadataEntry]);
    expect(result!.code).toContain("Bundle Isolation API");
  });
});
