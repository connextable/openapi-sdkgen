#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const targetByPlatform = {
  darwin: {
    arm64: "darwin-arm64",
    x64: "darwin-amd64",
  },
  linux: {
    arm64: "linux-arm64",
    x64: "linux-amd64",
  },
  win32: {
    arm64: "windows-arm64",
    x64: "windows-amd64",
  },
};

const target = targetByPlatform[process.platform]?.[process.arch];
if (target === undefined) {
  throw new Error(
    `openapi-sdkgen does not support ${process.platform}/${process.arch}. ` +
      "Supported platforms: darwin, linux, and win32 on arm64 or x64.",
  );
}

const packageDirectory = dirname(dirname(fileURLToPath(import.meta.url)));
const executable = join(
  packageDirectory,
  "bin",
  target,
  process.platform === "win32" ? "openapi-sdkgen.exe" : "openapi-sdkgen",
);

if (!existsSync(executable)) {
  throw new Error(`openapi-sdkgen executable is missing for ${target}. Reinstall the package.`);
}

const result = spawnSync(executable, process.argv.slice(2), { stdio: "inherit" });
if (result.error !== undefined) {
  throw result.error;
}

process.exitCode = result.status ?? 1;
