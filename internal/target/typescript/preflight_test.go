package typescript

import (
	"strings"
	"testing"

	sdkgen "github.com/connextable/openapi-sdkgen/internal/compiler"
	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/diagnostic"
	"github.com/connextable/openapi-sdkgen/internal/generator"
)

func TestPrepareAccumulatesIndependentTargetSupportDiagnostics(t *testing.T) {
	document := &ir.Document{
		Raw: map[string]any{
			"paths": map[string]any{
				"/things/{id}":   map[string]any{},
				"/things/{name}": map[string]any{},
			},
			"webhooks": map[string]any{"event": map[string]any{}},
		},
		ComponentSchemas: map[string]map[string]any{
			"Dynamic": {"$dynamicRef": "#node"},
		},
		Operations: []ir.Operation{
			{
				OperationID: "same",
				Method:      "GET",
				Path:        "/things/{id}",
				Raw: map[string]any{
					"security": "invalid",
					"responses": map[string]any{
						"200": map[string]any{"content": map[string]any{"text/event-stream": map[string]any{}}},
					},
				},
			},
			{OperationID: "same", Method: "GET", Path: "/things/{name}"},
		},
	}
	_, values, err := (Generator{}).Prepare(document, generator.Options{})
	if err != nil {
		t.Fatal(err)
	}
	report := diagnostic.RenderHuman(values, nil)
	for _, code := range []string{"SDKGEN-E501", "SDKGEN-E502", "SDKGEN-E503", "SDKGEN-E504", "SDKGEN-E505", "SDKGEN-E508"} {
		if !strings.Contains(report, code) {
			t.Fatalf("target preflight missing %s:\n%s", code, report)
		}
	}
}

func TestPrepareReplacesBaseInboundHintWithServerSemanticDiagnostics(t *testing.T) {
	document := &ir.Document{
		Raw: map[string]any{
			"webhooks": map[string]any{"bad": "not-a-path-item"},
		},
		Operations: []ir.Operation{{
			OperationID: "source",
			Method:      "POST",
			Path:        "/source",
			Raw: map[string]any{
				"callbacks": map[string]any{"bad": "not-a-callback"},
			},
		}},
	}
	_, baseValues, err := (Generator{}).Prepare(document, generator.Options{})
	if err != nil {
		t.Fatal(err)
	}
	baseReport := diagnostic.RenderHuman(baseValues, nil)
	if !strings.Contains(baseReport, "SDKGEN-E505") || !strings.Contains(baseReport, "--with server") {
		t.Fatalf("base inbound diagnostic =\n%s", baseReport)
	}

	options, err := generator.NewAddonRegistry(generator.AddonServer)
	if err != nil {
		t.Fatal(err)
	}
	serverOptions, err := options.Resolve([]string{"server"})
	if err != nil {
		t.Fatal(err)
	}
	_, serverValues, err := (Generator{}).Prepare(document, serverOptions)
	if err != nil {
		t.Fatal(err)
	}
	serverReport := diagnostic.RenderHuman(serverValues, nil)
	if strings.Contains(serverReport, "SDKGEN-E505") || strings.Count(serverReport, "SDKGEN-E506") < 2 {
		t.Fatalf("server inbound diagnostics =\n%s", serverReport)
	}
}

func TestPrepareAcceptsValidInboundContractsWithServerAddon(t *testing.T) {
	document, err := sdkgen.Compile([]byte(`{
  "openapi": "3.1.0",
  "info": {"title": "Inbound", "version": "1"},
  "paths": {},
  "webhooks": {
    "event": {
      "post": {
        "operationId": "receiveEvent",
        "requestBody": {
          "required": true,
          "content": {"application/json": {"schema": {"type": "object"}}}
        },
        "responses": {"204": {"description": "Accepted"}}
      }
    }
  }
}`))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := generator.NewAddonRegistry(generator.AddonServer)
	if err != nil {
		t.Fatal(err)
	}
	options, err := registry.Resolve([]string{"server"})
	if err != nil {
		t.Fatal(err)
	}
	_, values, err := (Generator{}).Prepare(document, options)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic.HasErrors(values) {
		t.Fatal(diagnostic.RenderHuman(values, nil))
	}
}
