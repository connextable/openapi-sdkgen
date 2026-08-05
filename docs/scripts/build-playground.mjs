import { copyFileSync, mkdirSync } from "node:fs";
import { resolve } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const docsDirectory = resolve(fileURLToPath(new URL("..", import.meta.url)));
const repositoryDirectory = resolve(docsDirectory, "..");
const outputDirectory = resolve(docsDirectory, "public", "playground");

mkdirSync(outputDirectory, { recursive: true });

const goRoot = spawnSync("go", ["env", "GOROOT"], {
  cwd: repositoryDirectory,
  encoding: "utf8",
});
if (goRoot.status !== 0) {
  process.stderr.write(goRoot.stderr);
  process.exit(goRoot.status ?? 1);
}

const build = spawnSync(
  "go",
  ["build", "-trimpath", "-ldflags=-s -w", "-o", resolve(outputDirectory, "openapi-sdkgen.wasm"), "./cmd/playground-wasm"],
  {
    cwd: repositoryDirectory,
    env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
    stdio: "inherit",
  },
);
if (build.status !== 0) {
  process.exit(build.status ?? 1);
}

copyFileSync(
  resolve(goRoot.stdout.trim(), "lib", "wasm", "wasm_exec.js"),
  resolve(outputDirectory, "wasm_exec.js"),
);
copyFileSync(
  resolve(docsDirectory, "scripts", "playground-worker.js"),
  resolve(outputDirectory, "generator-worker.js"),
);
