import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: ["tests/**/*.test.ts"],
  },
  coverage: {
    provider: "v8",
    include: ["fixtures/generated/client/generated/**/*.ts"],
    reporter: ["text", "json-summary", "lcov"],
    reportsDirectory: "../../.tmp/coverage/typescript",
    thresholds: {
      statements: 80,
      branches: 70,
      functions: 80,
      lines: 80,
    },
  },
});
