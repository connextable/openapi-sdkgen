import { describe, expect, it } from "vitest";

import {
  readPlaygroundPreferences,
  type PreferenceStorage,
  writePlaygroundPreferences,
} from "../../../docs/.vitepress/theme/playground/preferences.js";

function memoryStorage(initial: Record<string, string> = {}): PreferenceStorage {
  const values = new Map(Object.entries(initial));
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  };
}

describe("playground preferences", () => {
  const themes = ["github-dark", "nord"] as const;

  it("round-trips the code theme and sorted expanded directory paths", () => {
    const storage = memoryStorage();

    writePlaygroundPreferences(storage, {
      codeTheme: "nord",
      expandedPaths: new Set(["internal/routes", "internal"]),
    });

    expect(readPlaygroundPreferences(storage, themes)).toEqual({
      codeTheme: "nord",
      expandedPaths: ["internal", "internal/routes"],
    });
  });

  it("starts with every directory closed and ignores malformed expanded state", () => {
    const storage = memoryStorage({
      "openapi-sdkgen.playground.code-theme.v1": "unknown",
      "openapi-sdkgen.playground.expanded-paths.v1": "not-json",
    });

    expect(readPlaygroundPreferences(storage, themes)).toEqual({ expandedPaths: [] });
  });

  it("keeps preferences usable when browser storage is unavailable", () => {
    expect(readPlaygroundPreferences(undefined, themes)).toEqual({ expandedPaths: [] });
    expect(() =>
      writePlaygroundPreferences(undefined, {
        codeTheme: "github-dark",
        expandedPaths: new Set(),
      }),
    ).not.toThrow();
  });
});
