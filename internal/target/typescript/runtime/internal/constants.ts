/** Sort directions accepted by generated list operations. */
export const SortDirection = {
  /** Ascending sort order. */
  ASC: "asc",
  /** Descending sort order. */
  DESC: "desc",
} as const;

/** Sort direction value accepted by generated sort inputs. */
export type SortDirection = (typeof SortDirection)[keyof typeof SortDirection];
