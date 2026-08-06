package typescript

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdkgen "openapi-sdkgen/internal/compiler"
)

func TestVersionFeatureFixturesGenerateForTypeScript(t *testing.T) {
	for _, test := range []struct {
		fixture     string
		version     string
		want        string
		operationID string
		route       string
	}{
		{"oas30-sdk.json", "3.0.3", "string | null", "listWidgets", "GET /widgets"},
		{"oas31-sdk.json", "3.1.1", "string | null", "getWidget", "GET /widgets"},
		{"oas32-sdk.json", "3.2.0", `method: "QUERY"`, "queryWidgets", "QUERY /widgets"},
	} {
		t.Run(test.version, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "compiler", "openapi", "testdata", test.fixture))
			if err != nil {
				t.Fatal(err)
			}
			document, err := sdkgen.Compile(data)
			if err != nil {
				t.Fatal(err)
			}
			if document.OpenAPIVersion != test.version {
				t.Fatalf("version = %q, want %q", document.OpenAPIVersion, test.version)
			}
			typescriptArtifacts, err := SourceArtifacts(document)
			if err != nil {
				t.Fatal(err)
			}
			if source := string(artifactByPath(t, typescriptArtifacts, "internal/types.ts")) + string(artifactByPath(t, typescriptArtifacts, "internal/client.ts")); !strings.Contains(source, test.want) {
				t.Fatalf("TypeScript source missing %q:\n%s", test.want, source)
			}
			probe := fmt.Sprintf(`
import type { Client, OperationInput, OperationOutput, RouteInput, RouteOutput } from "./internal/client.js"
type Equal<Left, Right> = (<Value>() => Value extends Left ? 1 : 2) extends (<Value>() => Value extends Right ? 1 : 2) ? true : false
type Expect<Value extends true> = Value
type Method = Client["$operations"][%q]
type Assertions = [
  Expect<Equal<OperationInput<%q>, OperationInput<Method>>>,
  Expect<Equal<OperationInput<%q>, RouteInput<%q>>>,
  Expect<Equal<OperationOutput<%q>, RouteOutput<%q>>>,
]
void (null as unknown as Assertions)
`, test.operationID, test.operationID, test.operationID, test.route, test.operationID, test.route)
			compileTypeScriptArtifactsWithProbe(t, document, "operation-types.probe.ts", probe)
		})
	}
}
