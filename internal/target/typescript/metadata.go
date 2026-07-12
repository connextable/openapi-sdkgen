package typescript

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

// emitMetadata publishes the lossless OpenAPI document for fields that inform
// consumers (documentation, examples, tags, extensions) without changing a
// request's transport semantics. Executable features continue to use generated
// client code or a feature-path diagnostic.
func emitMetadata(document *ir.Document, typescript bool) ([]byte, error) {
	raw, err := json.Marshal(document.Raw)
	if err != nil {
		return nil, fmt.Errorf("encode OpenAPI metadata: %w", err)
	}
	var output bytes.Buffer
	output.WriteString("/** Lossless source OpenAPI document, including documentation, examples, and extensions. */\n")
	if typescript {
		fmt.Fprintf(&output, "export const openapiDocument = %s as const\n\n", raw)
	} else {
		fmt.Fprintf(&output, "export const openapiDocument = %s\n\n", raw)
	}
	fmt.Fprintf(&output, "/** Exact OpenAPI semantic version declared by the source document. */\nexport const openapiVersion = %s\n", quoteTS(document.OpenAPIVersion))
	fmt.Fprintf(&output, "/** OpenAPI minor line selected by the compiler. */\nexport const openapiVersionLine = %s\n", quoteTS(document.OpenAPIVersionLine))
	return output.Bytes(), nil
}
