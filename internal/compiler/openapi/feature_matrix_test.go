package openapi

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type featureManifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	States        []string          `json:"states"`
	Targets       []string          `json:"targets"`
	Features      []manifestFeature `json:"features"`
}

type manifestFeature struct {
	ID         string             `json:"id"`
	Versions   []string           `json:"versions"`
	State      string             `json:"state"`
	Evidence   string             `json:"evidence"`
	Conditions []featureCondition `json:"conditions"`
}

type featureCondition struct {
	Target   string   `json:"target"`
	With     []string `json:"with"`
	Scope    string   `json:"scope"`
	State    string   `json:"state"`
	Evidence string   `json:"evidence"`
}

func TestFeatureMatrixListsEveryVersionAndFeatureFamily(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "openapi-feature-matrix.md")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	for _, required := range []string{
		"OAS 3.0.4",
		"OAS 3.1.1",
		"OAS 3.2.0",
		"Version Detection and Top-level Objects",
		"Reuse, References, and Servers",
		"Paths, Operations, Parameters, and Request Bodies",
		"Responses, Media, Streams, Links, Callbacks, and Webhooks",
		"Components, Security, XML, and Metadata",
		"OAS 3.0.x Schema Object",
		"OAS 3.1.x and 3.2.x JSON Schema Draft 2020-12",
		"Version-only Deltas",
		"`querystring` parameter",
		"`additionalOperations`",
		"`$self`",
		"`$dynamicRef`",
		"Server-Sent Events",
		"Callback Object",
		"Webhook Object",
		"Mutual TLS",
		"Encoding Object",
	} {
		if !strings.Contains(string(contents), required) {
			t.Errorf("feature matrix is missing %q", required)
		}
	}

	for _, line := range strings.Split(string(contents), "\n") {
		status := strings.ToLower(line)
		if strings.HasPrefix(line, "| ") && (strings.Contains(status, "partial") || strings.Contains(status, "none")) {
			t.Errorf("feature matrix leaves an ambiguous current state: %s", line)
		}
	}

	lines := strings.Split(string(contents), "\n")
	root := filepath.Join("..", "..", "..")
	for index := 0; index < len(lines); index++ {
		header := markdownTableCells(lines[index])
		current := -1
		evidence := -1
		for column, value := range header {
			if value == "Current" {
				current = column
			}
			if value == "Evidence" {
				evidence = column
			}
		}
		if (current < 0 && evidence < 0) || index+1 >= len(lines) || !strings.HasPrefix(lines[index+1], "| ---") {
			continue
		}
		for index += 2; index < len(lines) && strings.HasPrefix(lines[index], "|"); index++ {
			cells := markdownTableCells(lines[index])
			if current >= 0 && current >= len(cells) {
				t.Errorf("feature matrix has no Current value: %s", lines[index])
			} else if current >= 0 {
				status := strings.ToLower(cells[current])
				if !strings.Contains(status, "generated") && !strings.Contains(status, "metadata") && !strings.Contains(status, "error") {
					t.Errorf("feature matrix Current value must map to generated, metadata, or error: %s", lines[index])
				}
			}
			if evidence < 0 || evidence >= len(cells) {
				t.Errorf("feature matrix row has no fixture/assertion evidence: %s", lines[index])
				continue
			}
			assertMatrixEvidence(t, root, cells[evidence], lines[index])
		}
		index--
	}
}

func TestAtomicFeatureInventoryUsesOneVerifiableTargetStatePerSurface(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "openapi-feature-inventory.md")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	for _, required := range []string{
		"Common Document and Discovery Surface",
		"Reuse and Components",
		"HTTP Calls, Parameters, and Bodies",
		"Responses, Asynchronous Features, and Security",
		"Schema Object: OAS 3.0.x",
		"Schema Object: JSON Schema 2020-12",
		"Version-only Syntax and Semantics",
	} {
		if !strings.Contains(string(contents), required) {
			t.Errorf("atomic feature inventory is missing %q", required)
		}
	}

	seenIDs := make(map[string]bool)
	lines := strings.Split(string(contents), "\n")
	root := filepath.Join("..", "..", "..")
	for index := 0; index < len(lines); index++ {
		header := markdownTableCells(lines[index])
		id, state, evidence := -1, -1, -1
		for column, value := range header {
			switch value {
			case "ID":
				id = column
			case "State":
				state = column
			case "Evidence":
				evidence = column
			}
		}
		if id < 0 || state < 0 || evidence < 0 || index+1 >= len(lines) || !strings.HasPrefix(lines[index+1], "| ---") {
			continue
		}
		for index += 2; index < len(lines) && strings.HasPrefix(lines[index], "|"); index++ {
			cells := markdownTableCells(lines[index])
			if id >= len(cells) || state >= len(cells) || evidence >= len(cells) {
				t.Errorf("atomic feature inventory has malformed row: %s", lines[index])
				continue
			}
			if cells[id] == "" || seenIDs[cells[id]] {
				t.Errorf("atomic feature inventory needs a unique ID: %s", lines[index])
			}
			seenIDs[cells[id]] = true
			switch cells[state] {
			case "generated", "metadata", "error":
			default:
				t.Errorf("atomic feature inventory needs exactly one target state: %s", lines[index])
			}
			assertMatrixEvidence(t, root, cells[evidence], lines[index])
		}
		index--
	}
	if len(seenIDs) < 45 {
		t.Errorf("atomic feature inventory has only %d surfaces, want at least 45", len(seenIDs))
	}
}

func TestCanonicalFeatureManifestHasEverySchemaKeywordAndExecutableEvidence(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "openapi-feature-manifest.json")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest featureManifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 1 || len(manifest.States) != 3 {
		t.Fatalf("unexpected feature manifest header: %#v", manifest)
	}
	if !sameStrings(manifest.Targets, []string{"typescript"}) {
		t.Fatalf("feature manifest targets = %#v, want TypeScript", manifest.Targets)
	}
	root := filepath.Join("..", "..", "..")
	seen := make(map[string]manifestFeature, len(manifest.Features))
	for _, feature := range manifest.Features {
		if feature.ID == "" || seen[feature.ID].ID != "" {
			t.Errorf("manifest feature ID must be unique: %#v", feature)
		}
		seen[feature.ID] = feature
		switch feature.State {
		case "generated", "metadata", "error":
		default:
			t.Errorf("manifest feature %q has invalid state %q", feature.ID, feature.State)
		}
		if len(feature.Versions) == 0 {
			t.Errorf("manifest feature %q has no versions", feature.ID)
		}
		for _, version := range feature.Versions {
			if version != "3.0" && version != "3.1" && version != "3.2" {
				t.Errorf("manifest feature %q has invalid version %q", feature.ID, version)
			}
		}
		assertMatrixEvidence(t, root, feature.Evidence, feature.ID)
		for _, condition := range feature.Conditions {
			if !containsString(manifest.Targets, condition.Target) {
				t.Errorf("manifest feature %q has invalid conditional target %q", feature.ID, condition.Target)
			}
			switch condition.State {
			case "generated", "metadata", "error":
			default:
				t.Errorf("manifest feature %q has invalid conditional state %q", feature.ID, condition.State)
			}
			if condition.Evidence == "" {
				t.Errorf("manifest feature %q has conditional state without evidence", feature.ID)
			} else {
				assertMatrixEvidence(t, root, condition.Evidence, feature.ID)
			}
		}
	}
	// The exact sorted feature/evidence contract is deliberate: neither an ID
	// nor its mapped target state, version scope, or assertion can be changed
	// without an explicit inventory-review update.
	manifestContracts := make([]string, 0, len(manifest.Features))
	for _, feature := range manifest.Features {
		manifestContracts = append(manifestContracts, strings.Join([]string{
			feature.ID, feature.State, strings.Join(feature.Versions, ","), feature.Evidence,
		}, "\t"))
		for _, condition := range feature.Conditions {
			if condition.Scope != "" && condition.Scope != "inbound-only" {
				t.Errorf("manifest feature %s has unsupported condition scope %q", feature.ID, condition.Scope)
			}
			manifestContracts = append(manifestContracts, strings.Join([]string{
				feature.ID + "@" + condition.Target, condition.Scope, condition.State, strings.Join(condition.With, ","), condition.Evidence,
			}, "\t"))
		}
	}
	sort.Strings(manifestContracts)
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(manifestContracts, "\n")))); got != "cec621bfaa6b6e6f37d4c2c2c16c8c7b9a27a5add7e90d2b58984342c8558190" {
		t.Errorf("manifest feature/evidence contract changed: %s", got)
	}

	for _, suffix := range []string{
		"title", "multipleOf", "maximum", "exclusiveMaximum", "minimum", "exclusiveMinimum",
		"maxLength", "minLength", "pattern", "maxItems", "minItems", "uniqueItems",
		"maxProperties", "minProperties", "required", "enum", "type", "allOf", "oneOf",
		"anyOf", "not", "items", "properties", "additionalProperties.schema",
		"additionalProperties.false", "description", "format", "default", "nullable", "discriminator",
		"readOnly", "writeOnly", "xml", "externalDocs", "example", "deprecated",
	} {
		if _, exists := seen["oas30.schema."+suffix]; !exists {
			t.Errorf("manifest misses OAS 3.0 Schema Object keyword %q", suffix)
		}
	}
	for _, suffix := range []string{
		"ref.direct-component", "ref.nested-pointer", "boolean", "id", "schema", "anchor", "dynamicAnchor",
		"dynamicRef", "vocabulary", "comment", "defs", "type", "const", "enum", "allOf", "anyOf", "oneOf",
		"not", "if", "then", "else", "prefixItems", "items", "contains", "minContains", "maxContains",
		"properties", "patternProperties", "additionalProperties.schema", "additionalProperties.false", "propertyNames",
		"unevaluatedItems", "unevaluatedProperties", "dependentSchemas", "dependentRequired", "contentSchema",
		"readOnly", "writeOnly", "discriminator", "xml", "multipleOf", "maximum", "exclusiveMaximum",
		"minimum", "exclusiveMinimum", "maxLength", "minLength", "pattern", "maxItems", "minItems",
		"uniqueItems", "maxProperties", "minProperties", "required", "title", "description", "default",
		"deprecated", "examples", "format", "contentEncoding", "contentMediaType", "externalDocs",
	} {
		if _, exists := seen["jsonschema."+suffix]; !exists {
			t.Errorf("manifest misses JSON Schema feature %q", suffix)
		}
	}
	for _, id := range []string{
		"root.openapi", "root.info", "root.servers.url", "root.servers.variables", "root.paths", "root.components",
		"root.security", "root.tags", "root.externalDocs", "root.extensions", "root.webhooks", "root.jsonSchemaDialect", "root.self",
		"components.schemas", "components.responses", "components.parameters", "components.examples", "components.requestBodies",
		"components.headers", "components.securitySchemes", "components.links", "components.callbacks", "components.pathItems", "components.mediaTypes",
		"operation.standard-methods", "operation.query", "operation.additionalOperations", "operation.operationId", "operation.tags", "operation.summary", "operation.description", "operation.externalDocs", "operation.deprecated",
		"operation.callbacks", "operation.security", "operation.scoped-server-alternatives",
		"parameter.in.path", "parameter.in.query", "parameter.in.header", "parameter.in.cookie", "parameter.querystring", "parameter.required", "parameter.allowReserved", "parameter.allowEmptyValue",
		"parameter.cookie-style", "parameter.styles", "parameter.delimited-object", "parameter.structured-non-json-content",
		"requestBody.media.json", "requestBody.media.text", "requestBody.media.binary", "requestBody.media.form-urlencoded", "requestBody.media.multipart", "requestBody.encoding", "media.xml", "media.itemSchema", "media.prefixEncoding", "media.itemEncoding",
		"response.status.exact", "response.status.default", "response.status.range", "response.default-media-negotiation", "response.headers", "response.links", "response.streams",
		"securityScheme.type", "securityScheme.name", "securityScheme.in", "securityScheme.scheme", "securityScheme.bearerFormat", "securityScheme.flows", "securityScheme.openIdConnectUrl",
		"info.contact.name", "info.contact.url", "info.contact.email", "info.license.name", "info.license.url",
		"server.url", "server.description", "server.name", "response.description", "response.summary", "response.content", "response.media-wildcard",
		"server.variable.enum", "server.variable.default", "server.variable.description",
		"tag.name", "tag.description", "tag.externalDocs", "tag.summary", "tag.parent", "tag.kind",
		"externalDocs.url", "externalDocs.description",
		"parameter.name", "parameter.in", "parameter.description", "parameter.deprecated", "parameter.style", "parameter.explode",
		"parameter.example", "parameter.examples", "mediaType.example", "mediaType.examples",
		"securityScheme.type", "securityScheme.name", "securityScheme.in", "securityScheme.scheme", "securityScheme.bearerFormat",
		"securityScheme.flows", "securityScheme.openIdConnectUrl", "securityScheme.oauth2MetadataUrl", "securityScheme.deprecated",
		"oauthFlow.authorizationUrl", "oauthFlow.tokenUrl", "oauthFlow.refreshUrl", "oauthFlow.scopes",
		"example.dataValue", "example.serializedValue",
		"input.yaml.flow-mapping", "pathItem.ref.local-pointer", "jsonschema.ref.sibling-wire-semantics",
		"media.streaming-request", "media.case-insensitive-binary",
	} {
		if _, exists := seen[id]; !exists {
			t.Errorf("manifest misses OpenAPI object feature %q", id)
		}
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func sameStrings(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for index, value := range expected {
		if actual[index] != value {
			return false
		}
	}
	return true
}

func assertMatrixEvidence(t *testing.T, root, evidence, row string) {
	t.Helper()
	reference := strings.Trim(evidence, "`")
	path, assertion, found := strings.Cut(reference, "::")
	if !found || path == "" || assertion == "" {
		t.Errorf("matrix evidence must be path::assertion: %s", row)
		return
	}
	contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Errorf("matrix evidence file %q is unreadable for row %s: %v", path, row, err)
		return
	}
	needle := "func " + assertion
	if filepath.Ext(path) != ".go" {
		needle = assertion
	}
	if !strings.Contains(string(contents), needle) {
		t.Errorf("matrix evidence assertion %q is absent from %q for row %s", assertion, path, row)
	}
}

func markdownTableCells(line string) []string {
	values := strings.Split(strings.Trim(line, "|"), "|")
	for index := range values {
		values[index] = strings.TrimSpace(values[index])
	}
	return values
}
