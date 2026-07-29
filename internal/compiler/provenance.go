package sdkgen

import (
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"go.yaml.in/yaml/v4"
)

func attachDocumentProvenance(document *ir.Document, source inputSource) {
	if document == nil {
		return
	}
	display := safeInputDisplay(source.display)
	document.Provenance = make(map[string]ir.Provenance)
	visitProvenanceNodes(document.Raw, nil, display, document.Provenance)
	for pointer, provenance := range referenceProvenance(source.data, display, source.fileBase) {
		document.Provenance[pointer] = provenance
	}
}

func visitProvenanceNodes(value any, path []string, source string, result map[string]ir.Provenance) {
	pointer := sourceJSONPointer(path)
	result[pointer] = ir.Provenance{Primary: ir.SourceLocation{Source: source, Pointer: pointer}}
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			visitProvenanceNodes(typed[key], append(path, key), source, result)
		}
	case []any:
		for index, child := range typed {
			visitProvenanceNodes(child, append(path, itoa(index)), source, result)
		}
	}
}

func referenceProvenance(data []byte, source, directory string) map[string]ir.Provenance {
	var value any
	if yaml.Unmarshal(data, &value) != nil {
		return nil
	}
	result := make(map[string]ir.Provenance)
	var visit func(any, []string)
	visit = func(current any, path []string) {
		switch typed := current.(type) {
		case map[string]any:
			if reference, _ := typed["$ref"].(string); reference != "" && !strings.HasPrefix(reference, "#") {
				name, fragment, _ := strings.Cut(reference, "#")
				targetSource := name
				if parsed, err := url.Parse(name); err == nil && parsed.IsAbs() {
					targetSource = safeInputDisplay(parsed.String())
				} else if directory != "" {
					if absolute, err := filepath.Abs(filepath.Join(directory, filepath.FromSlash(name))); err == nil {
						if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
							absolute = resolved
						}
						targetSource = absolute
					}
				}
				targetPointer := "#"
				if fragment != "" {
					if decoded, err := url.PathUnescape(fragment); err == nil {
						targetPointer += decoded
					}
				}
				pointer := sourceJSONPointer(path)
				result[pointer] = ir.Provenance{
					Primary: ir.SourceLocation{Source: targetSource, Pointer: targetPointer},
					Related: []ir.SourceLocation{{Source: source, Pointer: pointer + "/$ref"}},
				}
			}
			for key, child := range typed {
				visit(child, append(path, key))
			}
		case []any:
			for index, child := range typed {
				visit(child, append(path, itoa(index)))
			}
		}
	}
	visit(value, nil)
	return result
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}
