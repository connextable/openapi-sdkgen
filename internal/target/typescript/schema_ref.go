package typescript

import (
	"fmt"
	"net/url"
	"strings"
)

const componentSchemaReferencePrefix = "#/components/schemas/"

// componentSchemaReferenceName accepts only a complete local component-schema
// pointer. Nested JSON Pointers need a different target type/wire model and
// must be rejected before generation rather than being mistaken for a name.
func componentSchemaReferenceName(reference string) (string, error) {
	if !strings.HasPrefix(reference, "#") {
		return "", fmt.Errorf("unsupported schema reference %q", reference)
	}
	pointer, err := url.PathUnescape(strings.TrimPrefix(reference, "#"))
	if err != nil {
		return "", fmt.Errorf("schema reference %q has an invalid URI fragment escape", reference)
	}
	prefix := strings.TrimPrefix(componentSchemaReferencePrefix, "#")
	if !strings.HasPrefix(pointer, prefix) {
		return "", fmt.Errorf("unsupported schema reference %q", reference)
	}
	token := strings.TrimPrefix(pointer, prefix)
	if token == "" || strings.Contains(token, "/") {
		return "", fmt.Errorf("schema reference %q must target one component schema", reference)
	}
	var output strings.Builder
	for index := 0; index < len(token); index++ {
		if token[index] != '~' {
			output.WriteByte(token[index])
			continue
		}
		if index+1 >= len(token) {
			return "", fmt.Errorf("schema reference %q has an invalid JSON Pointer escape", reference)
		}
		index++
		switch token[index] {
		case '0':
			output.WriteByte('~')
		case '1':
			output.WriteByte('/')
		default:
			return "", fmt.Errorf("schema reference %q has an invalid JSON Pointer escape", reference)
		}
	}
	return output.String(), nil
}
