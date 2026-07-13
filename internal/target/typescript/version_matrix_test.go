package typescript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdkgen "github.com/connextable/openapi-sdkgen/internal/compiler"
)

func TestVersionFeatureFixturesGenerateForTypeScript(t *testing.T) {
	for _, test := range []struct {
		fixture string
		version string
		want    string
	}{
		{"oas30-sdk.json", "3.0.3", "string | null"},
		{"oas31-sdk.json", "3.1.1", "string | null"},
		{"oas32-sdk.json", "3.2.0", `method: "QUERY"`},
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
			if source := string(artifactByPath(t, typescriptArtifacts, "generated/types.ts")) + string(artifactByPath(t, typescriptArtifacts, "generated/client.ts")); !strings.Contains(source, test.want) {
				t.Fatalf("TypeScript source missing %q:\n%s", test.want, source)
			}
		})
	}
}
