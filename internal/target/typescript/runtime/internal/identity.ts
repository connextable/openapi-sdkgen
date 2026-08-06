declare const routeTypeBrand: unique symbol;

/** Internal route identity used to keep exact route surfaces distinct. */
export interface RouteTypeIdentity<Route> {
  readonly [routeTypeBrand]?: Route;
}

/** Internal operation surface identity. */
export type OperationSurface = "exact" | "resource";

declare const operationTypeBrand: unique symbol;

/** Internal operation identity used to keep exact and resource calls distinct. */
export interface OperationTypeIdentity<Route, Surface extends OperationSurface> {
  readonly [operationTypeBrand]?: {
    readonly route: Route;
    readonly surface: Surface;
  };
}
