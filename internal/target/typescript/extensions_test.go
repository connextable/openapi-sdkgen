package typescript

import (
	"strings"
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/compiler"
	"github.com/connextable/openapi-sdkgen/internal/diagnostic"
	"github.com/connextable/openapi-sdkgen/internal/generator"
)

func TestPrepareAccumulatesRecognizedExtensionDiagnosticsAndIgnoresInertMetadata(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "x-envelope": "data",
  "info": {"title": "Extension diagnostics", "version": "1"},
  "paths": {
    "/items": {"get": {
      "operationId": "listItems",
      "x-envelope": "none",
      "x-sdk-visibility": "public",
      "x-sort": {"format": "field-direction"},
      "x-filter": {"operator": "anything"},
      "x-concurrency": "required",
      "x-idempotency": "required",
      "parameters": [{
        "name": "sort",
        "in": "query",
        "schema": {"type": "array", "items": {"type": "string", "enum": ["bad"]}},
        "x-sort": {"format": "field-direction"}
      }],
      "responses": {"204": {"description": "OK"}}
    }}
  },
  "components": {"schemas": {
    "NotAnError": {"type": "string", "x-error-category": "wrong-place"}
  }}
}`))
	if err != nil {
		t.Fatal(err)
	}
	_, values, err := (Generator{}).Prepare(document, generator.Options{})
	if err != nil {
		t.Fatal(err)
	}
	counts := diagnostic.Count(values)
	if counts.Errors != 5 || counts.Warnings != 1 {
		t.Fatalf("counts = %#v, diagnostics = %#v", counts, values)
	}
	for _, code := range []string{"SDKGEN-E600", "SDKGEN-E611", "SDKGEN-E630", "SDKGEN-W620"} {
		if !diagnosticsContainCode(values, code) {
			t.Fatalf("missing %s: %#v", code, values)
		}
	}
	report := diagnostic.RenderHuman(values, nil)
	for _, inert := range []string{"x-filter", "x-concurrency", "x-idempotency"} {
		if strings.Contains(report, inert) {
			t.Fatalf("inert extension %s produced a diagnostic:\n%s", inert, report)
		}
	}
}

func TestEnvelopeDataProjectsOrdinaryOutputButKeepsRawBody(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "Envelope", "version": "1"},
  "paths": {"/value": {"get": {
    "operationId": "getValue",
    "x-envelope": "data",
    "responses": {
      "200": {"description": "OK", "content": {"application/json": {"schema": {
        "type": "object",
        "required": ["data", "meta"],
        "properties": {
          "data": {"type": "string"},
          "meta": {"type": "object", "required": ["trace"], "properties": {"trace": {"type": "string"}}}
        }
      }}}},
      "204": {"description": "No Content"}
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
	client := string(artifactByPath(t, artifacts, "generated/client.ts"))
	if !strings.Contains(client, `type `+operationTypeName("GET /value")+`Output = string | void`) {
		t.Fatalf("projected output missing:\n%s", client)
	}
	if !strings.Contains(client, `readonly "meta":`) {
		t.Fatalf("raw response lost complete envelope:\n%s", client)
	}
}

func TestEnvelopeDataRejectsEveryIncompatibleSuccessfulRepresentationTogether(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "Bad envelope", "version": "1"},
  "paths": {"/value": {"get": {
    "operationId": "getValue",
    "x-envelope": "data",
    "responses": {
      "200": {"description": "OK", "content": {
        "application/json": {"schema": {"type": "object", "properties": {"value": {"type": "string"}}}},
        "text/plain": {"schema": {"type": "string"}}
      }},
      "201": {"description": "Created", "content": {
        "application/octet-stream": {"schema": {"type": "string", "format": "binary"}}
      }}
    }
  }}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	_, values, err := (Generator{}).Prepare(document, generator.Options{})
	if err != nil {
		t.Fatal(err)
	}
	report := diagnostic.RenderHuman(values, nil)
	for _, expected := range []string{"SDKGEN-E612", "200 application/json", "200 text/plain", "201 application/octet-stream"} {
		if !strings.Contains(report, expected) {
			t.Fatalf("envelope report missing %q:\n%s", expected, report)
		}
	}
}

func TestSortWithoutExtensionKeepsExactStandardEnumInput(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "Standard sort", "version": "1"},
  "paths": {"/items": {"get": {
    "operationId": "listItems",
    "parameters": [
      {
        "name": "sort", "in": "query",
        "schema": {"type": "array", "items": {"type": "string", "enum": ["createdAt:asc", "createdAt:desc"]}}
      },
      {
        "name": "filter", "in": "query", "x-filter": {"operator": "domain-specific"},
        "schema": {"type": "string", "pattern": "^[a-z]+$"}
      }
    ],
    "responses": {"204": {"description": "OK"}}
  }}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	client := string(artifactByPath(t, artifacts, "generated/client.ts"))
	if !strings.Contains(client, `"createdAt:asc" | "createdAt:desc"`) || !strings.Contains(client, `readonly "filter"?: string | undefined`) || strings.Contains(client, `readonly field: "createdAt"`) {
		t.Fatalf("standard enum sort was projected without x-sort:\n%s", client)
	}
	metadata := string(artifactByPath(t, artifacts, "metadata.ts"))
	if !strings.Contains(metadata, `"x-filter"`) || !strings.Contains(metadata, `"domain-specific"`) {
		t.Fatalf("inert filter metadata was not preserved:\n%s", metadata)
	}
}

func TestVisibilityPlanKeepsInternalRoutesAndRemovesHiddenOperations(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "Visibility", "version": "1"},
  "paths": {
    "/public": {"get": {"operationId": "getPublic", "responses": {"204": {"description": "OK"}}}},
    "/internal": {"get": {"operationId": "getInternal", "x-sdk-visibility": "internal", "responses": {"204": {"description": "OK"}}}},
    "/hidden": {"get": {"operationId": "getHidden", "x-sdk-visibility": "hidden", "responses": {"204": {"description": "OK"}}}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	client := string(artifactByPath(t, artifacts, "generated/client.ts"))
	for _, expected := range []string{`readonly "GET /public":`, `readonly "GET /internal":`, `readonly "getInternal": Routes["GET /internal"]`} {
		if !strings.Contains(client, expected) {
			t.Fatalf("visibility surface missing %q:\n%s", expected, client)
		}
	}
	if strings.Contains(client, `readonly "GET /hidden":`) || strings.Contains(client, `readonly "getHidden":`) || strings.Contains(client, "readonly internal:") {
		t.Fatalf("visibility leaked hidden or internal resource operations:\n%s", client)
	}
}

func TestVisibleLinkCannotExposeHiddenTarget(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "Hidden dependency", "version": "1"},
  "paths": {
    "/source": {"get": {"operationId": "getSource", "responses": {"200": {
      "description": "OK", "links": {"hidden": {"operationId": "getHidden"}}
    }}}},
    "/hidden": {"get": {"operationId": "getHidden", "x-sdk-visibility": "hidden", "responses": {"204": {"description": "OK"}}}}
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	_, values, err := (Generator{}).Prepare(document, generator.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !diagnosticsContainCode(values, "SDKGEN-E621") {
		t.Fatalf("hidden dependency diagnostics = %#v", values)
	}
}

func TestErrorCategoryPreparationUsesWireCategoryAndAccumulatesConflicts(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "Categories", "version": "1"},
  "paths": {"/categories": {"get": {
    "responses": {
      "400": {"description": "Wire", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Wire"}}}},
      "401": {"description": "Redundant", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Redundant"}}}},
      "402": {"description": "Conflict", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Conflict"}}}},
      "403": {"description": "Optional", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Optional"}}}},
      "404": {"description": "Cross one", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/CrossOne"}}}},
      "405": {"description": "Cross two", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/CrossTwo"}}}}
    }
  }}},
  "components": {"schemas": {
    "Wire": {
      "type": "object", "required": ["error"],
      "properties": {"error": {
        "type": "object", "required": ["code", "category"],
        "properties": {"code": {"const": "wire"}, "category": {"const": "authentication"}}
      }}
    },
    "Redundant": {
      "type": "object", "x-error-category": "authentication", "required": ["error"],
      "properties": {"error": {
        "type": "object", "required": ["code", "category"],
        "properties": {"code": {"const": "redundant"}, "category": {"enum": ["authentication"]}}
      }}
    },
    "Conflict": {
      "type": "object", "x-error-category": "static", "required": ["error"],
      "properties": {"error": {
        "type": "object", "required": ["code", "category"],
        "properties": {"code": {"const": "conflict"}, "category": {"const": "wire"}}
      }}
    },
    "Optional": {
      "type": "object", "x-error-category": "static", "required": ["error"],
      "properties": {"error": {
        "type": "object", "required": ["code"],
        "properties": {"code": {"const": "optional"}, "category": {"enum": ["one", "two"]}}
      }}
    },
    "CrossOne": {
      "type": "object", "x-error-category": "one", "required": ["error"],
      "properties": {"error": {"type": "object", "required": ["code"], "properties": {"code": {"const": "shared"}}}}
    },
    "CrossTwo": {
      "type": "object", "x-error-category": "two", "required": ["error"],
      "properties": {"error": {"type": "object", "required": ["code"], "properties": {"code": {"const": "shared"}}}}
    }
  }}
}`))
	if err != nil {
		t.Fatal(err)
	}
	plan, values, err := prepareSourcePlan(document, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"SDKGEN-W641", "SDKGEN-E641", "SDKGEN-E642", "SDKGEN-E643"} {
		if !diagnosticsContainCode(values, code) {
			t.Fatalf("missing %s: %#v", code, values)
		}
	}
	if plan.document.ErrorCategories["Wire"] != "authentication" || plan.document.ErrorCategories["Redundant"] != "authentication" {
		t.Fatalf("wire categories = %#v", plan.document.ErrorCategories)
	}
}

func TestRequiredWireErrorCategoryGeneratesExactCategoryWithoutExtension(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "Wire category", "version": "1"},
  "paths": {"/session": {"get": {
    "operationId": "getSession",
    "responses": {
      "200": {"description": "OK"},
      "401": {"description": "Unauthorized", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/AuthError"}}}}
    }
  }}},
  "components": {"schemas": {
    "AuthError": {
      "allOf": [
        {"type": "object", "required": ["error"]},
        {"type": "object", "properties": {"error": {
          "allOf": [
            {"type": "object", "required": ["code", "category"]},
            {"type": "object", "properties": {
              "code": {"const": "authentication_required"},
              "category": {"const": "authentication-required"}
            }}
          ]
        }}}
      ]
    }
  }}
}`))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := SourceArtifacts(document)
	if err != nil {
		t.Fatal(err)
	}
	errorsSource := string(artifactByPath(t, artifacts, "generated/errors.ts"))
	if !strings.Contains(errorsSource, `readonly "authentication-required": "authentication_required"`) {
		t.Fatalf("wire error category missing:\n%s", errorsSource)
	}
}

func diagnosticsContainCode(values []diagnostic.Diagnostic, code string) bool {
	for _, value := range values {
		if value.Code == code {
			return true
		}
	}
	return false
}
