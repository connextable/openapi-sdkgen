/** Defines one enumerable own data property without prototype-setter semantics. */
export function defineOwnDataProperty<Value>(
  target: Record<string, Value>,
  key: string,
  value: Value,
): void {
  Object.defineProperty(target, key, {
    value,
    enumerable: true,
    configurable: true,
    writable: true,
  });
}

/** Reports whether a value is a non-array object record. */
export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
