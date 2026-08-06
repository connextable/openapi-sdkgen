import { isRecord } from "./objects.js";
import type { RequestOptions } from "./request.js";

/** Pagination strategy declared by an OpenAPI operation. */
export type PaginationProfile = "cursor" | "offset" | "both";

type QueryInput<Input> = Input extends { readonly query: infer Query } ? Query : never;
type WithoutQueryControl<Query, Name extends string> = Name extends ""
  ? Query
  : Omit<Query, Name> & { readonly [Key in Name]?: never };

/** Validated correlation between exact query controls and response-body locations. */
export type PaginationPlan<
  Profile extends PaginationProfile = PaginationProfile,
  CursorName extends string = string,
  OffsetName extends string = string,
> = {
  readonly mode: Profile;
  readonly request: {
    readonly cursor?: CursorName;
    readonly offset?: OffsetName;
    readonly limit?: string;
  };
  readonly response: {
    readonly items: readonly string[];
    readonly nextCursor?: readonly string[];
    readonly offset?: readonly string[];
    readonly limit?: readonly string[];
    readonly total?: readonly string[];
  };
};

/**
 * Input accepted by a generated pagination helper.
 *
 * Operations supporting both strategies require `mode`; cursor and offset fields
 * remain mutually exclusive in every profile.
 */
export type PaginateInput<
  Input,
  Profile extends PaginationProfile,
  CursorName extends string = "cursor",
  OffsetName extends string = "offset",
> = Profile extends "both"
  ?
      | (Omit<Input, "query"> & {
          readonly mode: "cursor";
          readonly query: WithoutQueryControl<QueryInput<Input>, OffsetName>;
        })
      | (Omit<Input, "query"> & {
          readonly mode: "offset";
          readonly query: WithoutQueryControl<QueryInput<Input>, CursorName>;
        })
  : Input & { readonly mode?: never };

/** Function that fetches one typed page for a generated pagination helper. */
export type PageRequest<Input, Page, Options extends RequestOptions = RequestOptions> = (
  input: Input,
  options?: Options,
) => Promise<Page>;

/**
 * Creates a lazy async iterator over all items returned by a paginated operation.
 *
 * The iterator preserves the original filters and sort order. Cursor pagination
 * advances only `cursor`; offset pagination advances only `offset`. No request is
 * sent until iteration begins, and iteration stops when the server signals the end.
 *
 * @param requestPage Generated function that fetches one page.
 * @param profile Pagination strategy declared by the operation.
 * @returns Function producing an {@link AsyncIterable} of decoded items.
 */
export function createPaginator<
  Item,
  Input,
  Page,
  Profile extends PaginationProfile = PaginationProfile,
  CursorName extends string = string,
  OffsetName extends string = string,
  Options extends RequestOptions = RequestOptions,
>(
  requestPage: PageRequest<Input, Page, Options>,
  plan: PaginationPlan<Profile, CursorName, OffsetName>,
): (
  input: PaginateInput<Input, Profile, CursorName, OffsetName>,
  options?: Options,
) => AsyncIterable<Item> {
  return (input, options) => ({
    async *[Symbol.asyncIterator]() {
      const root: Record<string, unknown> = isRecord(input) ? { ...input } : {};
      const requestedMode = root.mode;
      delete root.mode;
      const mode = resolvePaginationMode(plan.mode, requestedMode);
      const query = isRecord(root.query) ? { ...root.query } : {};
      const cursorName = plan.request.cursor;
      const offsetName = plan.request.offset;
      const limitName = plan.request.limit;
      if (mode === "cursor" && offsetName !== undefined && query[offsetName] !== undefined) {
        throw new TypeError(`cursor pagination does not accept ${offsetName}`);
      }
      if (mode === "offset" && cursorName !== undefined && query[cursorName] !== undefined) {
        throw new TypeError(`offset pagination does not accept ${cursorName}`);
      }
      root.query = query;
      const seenCursors = new Set<string>();
      if (cursorName !== undefined && typeof query[cursorName] === "string") {
        seenCursors.add(query[cursorName]);
      }
      const seenOffsets = new Set<number>();
      if (offsetName !== undefined && typeof query[offsetName] === "number") {
        seenOffsets.add(query[offsetName]);
      }
      for (;;) {
        const page = await requestPage({ ...root, query: { ...query } } as Input, options);
        const items = pageItems(page, plan.response.items);
        for (const item of items) yield item as Item;
        if (mode === "cursor") {
          const nextCursor = paginationValue(page, plan.response.nextCursor);
          if (typeof nextCursor !== "string" || nextCursor === "" || seenCursors.has(nextCursor)) {
            return;
          }
          seenCursors.add(nextCursor);
          if (cursorName === undefined) return;
          query[cursorName] = nextCursor;
          continue;
        }
        if (offsetName === undefined) return;
        const requestedOffset = numberValue(query[offsetName], undefined, 0);
        const currentOffset = numberValue(
          paginationValue(page, plan.response.offset),
          query[offsetName],
          0,
        );
        const limit = numberValue(
          paginationValue(page, plan.response.limit),
          limitName === undefined ? undefined : query[limitName],
          items.length,
        );
        const totalValue = paginationValue(page, plan.response.total);
        const total = typeof totalValue === "number" ? totalValue : undefined;
        const nextOffset = currentOffset + limit;
        if (
          limit <= 0 ||
          items.length === 0 ||
          items.length < limit ||
          nextOffset <= requestedOffset ||
          seenOffsets.has(nextOffset) ||
          (total !== undefined && nextOffset >= total)
        )
          return;
        seenOffsets.add(nextOffset);
        query[offsetName] = nextOffset;
      }
    },
  });
}

function resolvePaginationMode(
  profile: PaginationProfile,
  requested: unknown,
): "cursor" | "offset" {
  if (profile === "both") {
    if (requested !== "cursor" && requested !== "offset") {
      throw new TypeError('Pagination profile "both" requires mode "cursor" or "offset"');
    }
    return requested;
  }
  if (requested !== undefined && requested !== profile) {
    throw new TypeError(`Pagination mode ${String(requested)} does not match ${profile}`);
  }
  return profile;
}

function pageItems(page: unknown, pointer: readonly string[]): readonly unknown[] {
  const value = paginationValue(page, pointer);
  return Array.isArray(value) ? value : [];
}

function paginationValue(page: unknown, pointer: readonly string[] | undefined): unknown {
  if (pointer === undefined) return undefined;
  let current = page;
  for (const token of pointer) {
    if ((typeof current !== "object" && typeof current !== "function") || current === null) {
      return undefined;
    }
    if (!Object.prototype.hasOwnProperty.call(current, token)) {
      return undefined;
    }
    current = (current as Record<string, unknown>)[token];
  }
  return current;
}

function numberValue(primary: unknown, secondary: unknown, fallback: number): number {
  if (typeof primary === "number" && Number.isFinite(primary)) return primary;
  if (typeof secondary === "number" && Number.isFinite(secondary)) return secondary;
  return fallback;
}
