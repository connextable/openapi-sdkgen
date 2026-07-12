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
	output.WriteString("/** Lossless OpenAPI metadata, kept separate from the client call surface. */\n")
	if typescript {
		fmt.Fprintf(&output, "export const openapi = { document: %s, version: %s, versionLine: %s } as const\n", raw, quoteTS(document.OpenAPIVersion), quoteTS(document.OpenAPIVersionLine))
	} else {
		fmt.Fprintf(&output, "export const openapi = Object.freeze({ document: %s, version: %s, versionLine: %s })\n", raw, quoteTS(document.OpenAPIVersion), quoteTS(document.OpenAPIVersionLine))
	}
	return output.Bytes(), nil
}
