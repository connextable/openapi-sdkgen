package sdkgen

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	dynamicAnchorMetadataKey    = "x-sdkgen-dynamic-anchor"
	dynamicReferenceMetadataKey = "x-sdkgen-dynamic-reference"
)

// normalizeNestedSchemaReferences lowers local pointers below a component
// schema to their schema value before targets run. Component-root references
// remain named references, preserving recursive TypeScript declarations.
func normalizeNestedSchemaReferences(document any) (any, error) {
	root, ok := document.(map[string]any)
	if !ok {
		return document, nil
	}
	anchored, err := normalizeSchemaAnchorReferences(root)
	if err != nil {
		return nil, err
	}
	root, ok = anchored.(map[string]any)
	if !ok {
		return anchored, nil
	}
	components, _ := root["components"].(map[string]any)
	componentSchemas, _ := components["schemas"].(map[string]any)
	if len(componentSchemas) == 0 {
		return document, nil
	}
	return normalizeSchemaValue(root, root, map[string]bool{})
}

// normalizeSchemaAnchorReferences resolves local JSON Schema anchors before
// target lowering. TypeScript's wire descriptor model addresses schemas by
// JSON Pointer, so keeping an anchor or a dynamic reference until target
// emission would silently discard its reference semantics. A dynamic
// reference whose dynamic scope is wholly represented in this document is
// equivalent to the canonical pointer selected here; references that cannot
// be bound locally remain an explicit compilation error.
func normalizeSchemaAnchorReferences(document map[string]any) (any, error) {
	index := schemaAnchorIndex{anchors: map[string]string{}}
	base := schemaDocumentResourceURI(document)
	if err := index.collect(document, "", base); err != nil {
		return nil, err
	}
	return index.rewrite(document, "", base)
}

type schemaAnchorIndex struct {
	anchors map[string]string
}

func (index schemaAnchorIndex) collect(value any, pointer, resource string) error {
	switch typed := value.(type) {
	case map[string]any:
		current := resource
		if schemaObjectCandidate(typed) {
			if identifier, _ := typed["$id"].(string); identifier != "" {
				current = resolveSchemaResourceURI(resource, identifier)
			}
			for _, keyword := range []string{"$anchor", "$dynamicAnchor"} {
				anchor, _ := typed[keyword].(string)
				if anchor == "" {
					continue
				}
				name := schemaAnchorURI(current, anchor)
				if existing, exists := index.anchors[name]; exists && existing != "#"+pointer {
					return fmt.Errorf("duplicate JSON Schema anchor %q at %s and %s", anchor, existing, "#"+pointer)
				}
				index.anchors[name] = "#" + pointer
			}
		}
		for key, child := range typed {
			if schemaReferenceLiteralKey(key) {
				continue
			}
			if err := index.collect(child, appendSchemaPointer(pointer, key), current); err != nil {
				return err
			}
		}
	case []any:
		for offset, child := range typed {
			if err := index.collect(child, appendSchemaPointer(pointer, fmt.Sprint(offset)), resource); err != nil {
				return err
			}
		}
	}
	return nil
}

func (index schemaAnchorIndex) rewrite(value any, pointer, resource string) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		current := resource
		candidate := schemaObjectCandidate(typed)
		if candidate {
			if identifier, _ := typed["$id"].(string); identifier != "" {
				current = resolveSchemaResourceURI(resource, identifier)
			}
		}
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			if candidate && key == "$anchor" {
				continue
			}
			if candidate && key == "$dynamicRef" {
				reference, _ := child.(string)
				resolved, found := index.resolve(current, reference)
				if !found {
					return nil, fmt.Errorf("unresolved local JSON Schema dynamic reference %q at #%s", reference, pointer)
				}
				anchor, _ := schemaReferenceAnchor(reference)
				result[dynamicReferenceMetadataKey] = map[string]any{"anchor": anchor, "reference": resolved}
				continue
			}
			if candidate && key == "$dynamicAnchor" {
				if anchor, _ := child.(string); anchor != "" {
					result[dynamicAnchorMetadataKey] = anchor
				}
				continue
			}
			if candidate && key == "$ref" {
				reference, _ := child.(string)
				resolved, found := index.resolve(current, reference)
				if found {
					result[key] = resolved
					continue
				}
			}
			if schemaReferenceLiteralKey(key) {
				result[key] = child
				continue
			}
			normalized, err := index.rewrite(child, appendSchemaPointer(pointer, key), current)
			if err != nil {
				return nil, err
			}
			result[key] = normalized
		}
		return result, nil
	case []any:
		result := make([]any, len(typed))
		for offset, child := range typed {
			normalized, err := index.rewrite(child, appendSchemaPointer(pointer, fmt.Sprint(offset)), resource)
			if err != nil {
				return nil, err
			}
			result[offset] = normalized
		}
		return result, nil
	default:
		return value, nil
	}
}

func schemaReferenceAnchor(reference string) (string, bool) {
	_, anchor, found := strings.Cut(reference, "#")
	if !found || anchor == "" || strings.Contains(anchor, "/") {
		return "", false
	}
	return anchor, true
}

func (index schemaAnchorIndex) resolve(resource, reference string) (string, bool) {
	if reference == "" || strings.Contains(reference, "#/") {
		return "", false
	}
	fragment := ""
	if before, after, ok := strings.Cut(reference, "#"); ok {
		if after == "" || strings.Contains(after, "/") {
			return "", false
		}
		fragment = after
		resource = resolveSchemaResourceURI(resource, before)
	} else {
		return "", false
	}
	resolved, ok := index.anchors[schemaAnchorURI(resource, fragment)]
	return resolved, ok
}

func schemaObjectCandidate(value map[string]any) bool {
	for _, key := range []string{
		"$id", "$schema", "$anchor", "$dynamicAnchor", "$dynamicRef", "$defs", "$ref",
		"type", "properties", "items", "allOf", "anyOf", "oneOf", "not", "if", "then", "else",
	} {
		if _, exists := value[key]; exists {
			return true
		}
	}
	return false
}

func schemaDocumentResourceURI(document map[string]any) string {
	for _, key := range []string{"$self", "$id"} {
		if value, _ := document[key].(string); value != "" {
			return strings.TrimSuffix(strings.SplitN(value, "#", 2)[0], "#")
		}
	}
	return "urn:openapi-sdkgen:document"
}

func schemaAnchorURI(resource, anchor string) string {
	return strings.SplitN(resource, "#", 2)[0] + "#" + anchor
}

func resolveSchemaResourceURI(base, identifier string) string {
	if identifier == "" {
		return strings.SplitN(base, "#", 2)[0]
	}
	if strings.HasPrefix(identifier, "#") {
		return strings.SplitN(base, "#", 2)[0] + identifier
	}
	baseURL, baseErr := url.Parse(strings.SplitN(base, "#", 2)[0])
	identifierURL, identifierErr := url.Parse(identifier)
	if baseErr != nil || identifierErr != nil || strings.HasPrefix(base, "urn:") {
		return identifier
	}
	return baseURL.ResolveReference(identifierURL).String()
}

func appendSchemaPointer(pointer, token string) string {
	encoded := strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
	return pointer + "/" + encoded
}

func normalizeSchemaValue(value any, root map[string]any, resolving map[string]bool) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		if reference, _ := typed["$ref"].(string); reference != "" {
			if resolved, nested, err := resolveNestedComponentSchemaReference(root, reference); err != nil {
				return nil, err
			} else if nested {
				if resolving[reference] {
					return nil, fmt.Errorf("cyclic nested schema reference %q", reference)
				}
				resolving[reference] = true
				defer delete(resolving, reference)
				resolved, err = normalizeSchemaValue(resolved, root, resolving)
				if err != nil {
					return nil, err
				}
				siblings := make(map[string]any, len(typed)-1)
				for key, child := range typed {
					if key != "$ref" {
						siblings[key] = child
					}
				}
				if len(siblings) == 0 {
					return resolved, nil
				}
				return map[string]any{"allOf": []any{resolved, siblings}}, nil
			}
		}
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			if schemaReferenceLiteralKey(key) {
				result[key] = child
				continue
			}
			normalized, err := normalizeSchemaValue(child, root, resolving)
			if err != nil {
				return nil, err
			}
			result[key] = normalized
		}
		return result, nil
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			normalized, err := normalizeSchemaValue(child, root, resolving)
			if err != nil {
				return nil, err
			}
			result[index] = normalized
		}
		return result, nil
	default:
		return value, nil
	}
}

func schemaReferenceLiteralKey(key string) bool {
	switch key {
	case "const", "default", "enum", "example", "examples", "value", "dataValue", "serializedValue":
		return true
	default:
		return false
	}
}

func resolveNestedComponentSchemaReference(root map[string]any, reference string) (any, bool, error) {
	const prefix = "#/components/schemas/"
	if !strings.HasPrefix(reference, prefix) {
		return nil, false, nil
	}
	tokens := strings.Split(strings.TrimPrefix(reference, "#/"), "/")
	if len(tokens) <= 3 {
		return nil, false, nil
	}
	var value any = root
	for _, token := range tokens {
		decoded, err := decodeJSONPointerToken(token)
		if err != nil {
			return nil, false, fmt.Errorf("schema reference %q: %w", reference, err)
		}
		switch current := value.(type) {
		case map[string]any:
			var exists bool
			value, exists = current[decoded]
			if !exists {
				return nil, false, fmt.Errorf("unresolved nested schema reference %q", reference)
			}
		case []any:
			return nil, false, fmt.Errorf("nested schema reference %q cannot target an array index", reference)
		default:
			return nil, false, fmt.Errorf("unresolved nested schema reference %q", reference)
		}
	}
	if _, ok := value.(map[string]any); !ok {
		if _, ok := value.(bool); !ok {
			return nil, false, fmt.Errorf("nested schema reference %q does not target a Schema Object", reference)
		}
	}
	return value, true, nil
}

func decodeJSONPointerToken(token string) (string, error) {
	var result strings.Builder
	for index := 0; index < len(token); index++ {
		if token[index] != '~' {
			result.WriteByte(token[index])
			continue
		}
		if index+1 >= len(token) {
			return "", fmt.Errorf("invalid JSON Pointer escape")
		}
		index++
		switch token[index] {
		case '0':
			result.WriteByte('~')
		case '1':
			result.WriteByte('/')
		default:
			return "", fmt.Errorf("invalid JSON Pointer escape")
		}
	}
	return result.String(), nil
}
