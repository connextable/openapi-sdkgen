package typescript

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	sdkgen "github.com/connextable/openapi-sdkgen/internal/compiler"
	"github.com/connextable/openapi-sdkgen/internal/diagnostic"
)

func TestHermeticAfterSalesPaginationFixtureGenerates(t *testing.T) {
	source, err := os.ReadFile("testdata/after-sales.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	document, err := sdkgen.Compile(source)
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	client := string(artifactByPath(t, artifacts, "generated/client.ts"))
	for _, expected := range []string{
		`readonly "GET /after-sales-requests":`,
		`readonly paginate:`,
		`AsyncIterable<Contract.ComponentOutput<"AfterSalesAdminListItem">>`,
		`["response", Object.fromEntries([["items", ["data"]], ["nextCursor", ["meta","pagination","nextCursor"]]])]`,
	} {
		if !strings.Contains(client, expected) {
			t.Fatalf("after-sales pagination output missing %q:\n%s", expected, client)
		}
	}
}

func TestPaginationShorthandPlansEveryCanonicalLayout(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "Canonical pagination", "version": "1"},
  "paths": {
    "/root": {"get": {
      "operationId": "root",
      "parameters": [
        {"name": "cursor", "in": "query", "schema": {"type": "string"}},
        {"name": "limit", "in": "query", "schema": {"type": "integer", "minimum": 1}}
      ],
      "responses": {
        "200": {"description": "OK", "content": {"application/json": {"schema": {
          "type": "object", "properties": {
            "items": {"type": "array", "items": {"$ref": "#/components/schemas/Item"}},
            "pagination": {"type": "object", "properties": {"nextCursor": {"type": ["string", "null"]}}}
          }
        }}}},
        "201": {"description": "Also OK", "content": {"application/json": {"schema": {
          "type": "object", "properties": {
            "items": {"type": "array", "items": {"type": "string"}},
            "pagination": {"type": "object", "properties": {"nextCursor": {"type": "string"}}}
          }
        }}}}
      },
      "x-pagination": "cursor"
    }},
    "/nested": {"get": {
      "operationId": "nested",
      "parameters": [
        {"name": "cursor", "in": "query", "schema": {"type": "string"}},
        {"name": "limit", "in": "query", "schema": {"type": "integer", "minimum": 1}}
      ],
      "responses": {"200": {"description": "OK", "content": {"application/json": {"schema": {
        "type": "object", "properties": {"data": {
          "type": "object", "properties": {
            "items": {"type": "array", "items": {"type": "string"}},
            "pagination": {"type": "object", "properties": {"nextCursor": {"type": ["string", "null"]}}}
          }
        }}
      }}}}},
      "x-pagination": "cursor"
    }},
    "/data": {"get": {
      "operationId": "data",
      "parameters": [
        {"name": "cursor", "in": "query", "schema": {"type": "string"}},
        {"name": "limit", "in": "query", "schema": {"type": "integer", "minimum": 1}}
      ],
      "responses": {"200": {"description": "OK", "content": {"application/json": {"schema": {
        "type": "object", "properties": {
          "data": {"type": "array", "items": {"type": "string"}},
          "meta": {"type": "object", "properties": {"pagination": {
            "type": "object", "properties": {"nextCursor": {"type": ["string", "null"]}}
          }}}
        }
      }}}}},
      "x-pagination": "cursor"
    }}
  },
  "components": {"schemas": {"Item": {"type": "string"}}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	plan, values, err := prepareSourcePlan(document, false)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic.HasErrors(values) {
		t.Fatal(diagnostic.RenderHuman(values, nil))
	}
	expected := map[string][]string{
		"GET /root":   {"items"},
		"GET /nested": {"data", "items"},
		"GET /data":   {"data"},
	}
	for _, operation := range plan.document.Operations {
		if operation.PaginationPlan == nil {
			t.Fatalf("%s has no pagination plan", operation.RouteKey)
		}
		if !reflect.DeepEqual(operation.PaginationPlan.Response.Items, expected[operation.RouteKey]) {
			t.Fatalf("%s items = %#v", operation.RouteKey, operation.PaginationPlan.Response.Items)
		}
	}
}

func TestPaginationPreparationAccumulatesRequestResponseAndAmbiguityDiagnostics(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "Invalid pagination", "version": "1"},
  "paths": {
    "/invalid": {"get": {
      "operationId": "invalid",
      "parameters": [
        {"name": "cursor", "in": "query", "schema": {"type": "integer"}},
        {"name": "offset", "in": "query", "schema": {"type": "integer"}},
        {"name": "limit", "in": "query", "schema": {"type": "integer", "minimum": 0}}
      ],
      "responses": {"200": {"description": "OK", "content": {"application/json": {"schema": {
        "type": "object", "properties": {
          "items": {"type": "array", "items": {"type": "string"}},
          "pagination": {"type": "object", "properties": {
            "nextCursor": {"type": "integer"},
            "offset": {"type": "integer", "minimum": -1},
            "limit": {"type": "integer", "minimum": 0},
            "total": {"type": "number", "minimum": 0}
          }}
        }
      }}}}},
      "x-pagination": {
        "mode": "both",
        "request": {"cursor": "cursor", "offset": "offset", "limit": "limit"},
        "response": {
          "items": "/missing",
          "nextCursor": "/pagination/nextCursor",
          "offset": "/pagination/offset",
          "limit": "/pagination/limit",
          "total": "/pagination/total"
        }
      }
    }},
    "/ambiguous": {"get": {
      "operationId": "ambiguous",
      "parameters": [
        {"name": "cursor", "in": "query", "schema": {"type": "string"}},
        {"name": "limit", "in": "query", "schema": {"type": "integer", "minimum": 1}}
      ],
      "responses": {"200": {"description": "OK", "content": {"application/json": {"schema": {
        "type": "object", "properties": {
          "items": {"type": "array", "items": {"type": "string"}},
          "pagination": {"type": "object", "properties": {"nextCursor": {"type": "string"}}},
          "data": {"type": "object", "properties": {
            "items": {"type": "array", "items": {"type": "string"}},
            "pagination": {"type": "object", "properties": {"nextCursor": {"type": "string"}}}
          }}
        }
      }}}}},
      "x-pagination": "cursor"
    }}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	_, values, err := prepareSourcePlan(document, false)
	if err != nil {
		t.Fatal(err)
	}
	report := diagnostic.RenderHuman(values, nil)
	for _, expected := range []string{
		"SDKGEN-E651", "SDKGEN-E653", "SDKGEN-E654",
		"must declare a positive integer bound", "does not resolve to an array",
		"Pagination offset pointer", "Pagination limit pointer", "Pagination total pointer",
		"ambiguous",
	} {
		if !strings.Contains(report, expected) {
			t.Fatalf("pagination diagnostics missing %q:\n%s", expected, report)
		}
	}
}

func TestGeneratedExplicitPaginationPreservesCustomControlsEnvelopeAndQueryMode(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "Explicit pagination runtime", "version": "1"},
  "paths": {"/things": {"get": {
    "operationId": "listThings",
    "x-envelope": "data",
    "parameters": [
      {"name": "cursorToken", "in": "query", "schema": {"type": "string"}},
      {"name": "pageOffset", "in": "query", "schema": {"type": "integer", "minimum": 0}},
      {"name": "pageSize", "in": "query", "schema": {"type": "integer", "minimum": 1}},
      {"name": "mode", "in": "query", "required": true, "schema": {"type": "string"}},
      {"name": "filter", "in": "query", "required": true, "schema": {"type": "string"}}
    ],
    "responses": {"200": {"description": "OK", "content": {"application/json": {"schema": {
      "type": "object", "required": ["data", "meta"],
      "properties": {
        "data": {"type": "array", "items": {
          "type": "object", "required": ["id"], "properties": {"id": {"type": "string"}}
        }},
        "meta": {"type": "object", "required": ["pagination"], "properties": {"pagination": {
          "type": "object",
          "properties": {
            "nextCursor": {"type": ["string", "null"]},
            "offset": {"type": ["integer", "null"], "minimum": 0},
            "limit": {"type": ["integer", "null"], "minimum": 1},
            "total": {"type": ["integer", "null"], "minimum": 0}
          }
        }}}
      }
    }}}}},
    "x-pagination": {
      "mode": "both",
      "request": {"cursor": "cursorToken", "offset": "pageOffset", "limit": "pageSize"},
      "response": {
        "items": "/data",
        "nextCursor": "/meta/pagination/nextCursor",
        "offset": "/meta/pagination/offset",
        "limit": "/meta/pagination/limit",
        "total": "/meta/pagination/total"
      }
    }
  }}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	metadata := string(artifactByPath(t, artifacts, "metadata.ts"))
	for _, expected := range []string{`"x-pagination"`, `"cursorToken"`, `"/meta/pagination/nextCursor"`} {
		if !strings.Contains(metadata, expected) {
			t.Fatalf("explicit pagination metadata missing %q:\n%s", expected, metadata)
		}
	}
	probe := `import { createClient } from "./index.js"
declare const api: ReturnType<typeof createClient>
api.$operations.listThings.paginate({ mode: "cursor", query: { mode: "wire", filter: "kept", pageSize: 2 } })
api.$operations.listThings.paginate({ mode: "offset", query: { mode: "wire", filter: "kept", pageOffset: 0, pageSize: 2 } })
// @ts-expect-error cursor mode excludes the mapped offset control
api.$operations.listThings.paginate({ mode: "cursor", query: { mode: "wire", filter: "kept", pageOffset: 0, pageSize: 2 } })
`
	output := compileTypeScriptArtifactsWithProbe(t, document, "pagination.probe.ts", probe)
	script := `
import { pathToFileURL } from "node:url";
const { createClient } = await import(pathToFileURL(process.argv[1]).href);
const seen = [];
const api = createClient({ baseURL: "https://api.example.test", fetch: async (input) => {
  const url = new URL(String(input));
  const query = Object.fromEntries(url.searchParams);
  seen.push(query);
  if (query.cursorToken === undefined && query.pageOffset === undefined) {
    return Response.json({ data: [{ id: "c1" }], meta: { pagination: { nextCursor: "next" } } });
  }
  if (query.cursorToken === "next") {
    return Response.json({ data: [{ id: "c2" }], meta: { pagination: { nextCursor: null } } });
  }
  const offset = Number(query.pageOffset);
  return Response.json({
    data: offset === 0 ? [{ id: "o1" }, { id: "o2" }] : [{ id: "o3" }],
    meta: { pagination: { offset, limit: 2, total: 3 } },
  });
} });
const collect = async (iterable) => { const values = []; for await (const value of iterable) values.push(value.id); return values; };
const cursor = await collect(api.$operations.listThings.paginate({ mode: "cursor", query: { mode: "wire", filter: "kept", pageSize: 2 } }));
const offset = await collect(api.$operations.listThings.paginate({ mode: "offset", query: { mode: "wire", filter: "kept", pageOffset: 0, pageSize: 2 } }));
if (cursor.join(",") !== "c1,c2" || offset.join(",") !== "o1,o2,o3") throw new Error("pagination items mismatch");
if (seen.some((query) => query.mode !== "wire" || query.filter !== "kept" || Object.hasOwn(query, "mode") === false)) throw new Error("ordinary query values were not preserved");
if (seen[1].cursorToken !== "next" || seen[3].pageOffset !== "2") throw new Error("custom controls did not advance: " + JSON.stringify(seen));
const raw = await api.$operations.listThings.raw({ query: { mode: "wire", filter: "kept", pageSize: 2 } });
if (!Array.isArray(raw.data.data) || raw.data.meta.pagination.nextCursor !== "next") throw new Error("raw envelope metadata was lost");
`
	if output, err := exec.Command("node", "--input-type=module", "--eval", script, filepath.Join(output, "index.js")).CombinedOutput(); err != nil {
		t.Fatalf("execute explicit pagination runtime test: %v\n%s", err, output)
	}
}
