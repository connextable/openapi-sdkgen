package sdkgen

import (
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"go.yaml.in/yaml/v4"
)

func attachDocumentProvenance(document *ir.Document, source inputSource) {
	attachDocumentProvenanceWithSources(document, source, nil)
}

func attachDocumentProvenanceWithSources(document *ir.Document, source inputSource, remoteSources map[string][]byte) {
	if document == nil {
		return
	}
	display := safeInputDisplay(source.display)
	document.Provenance = make(map[string]ir.Provenance)
	visitProvenanceNodes(document.Raw, nil, display, document.Provenance)
	references := referenceProvenance(source.data, display, source.effective, source.fileBase, remoteSources)
	for pointer, provenance := range references {
		document.Provenance[pointer] = provenance
	}
	propagateReferenceProvenance(document.Raw, references, document.Provenance)
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

func referenceProvenance(data []byte, displaySource, resolutionSource, directory string, remoteSources map[string][]byte) map[string]ir.Provenance {
	var value any
	if yaml.Unmarshal(data, &value) != nil {
		return nil
	}
	if resolutionSource == "" {
		resolutionSource = displaySource
	}
	result := make(map[string]ir.Provenance)
	visiting := make(map[string]bool)
	var visit func(any, []string, string, string, string, string)
	visit = func(current any, path []string, currentSource, currentDisplay, currentDirectory, currentPointer string) {
		switch typed := current.(type) {
		case map[string]any:
			if reference, _ := typed["$ref"].(string); reference != "" && !strings.HasPrefix(reference, "#") {
				name, fragment, _ := strings.Cut(reference, "#")
				targetSource := name
				targetDirectory := ""
				if parsed, err := url.Parse(name); err == nil && parsed.IsAbs() {
					targetSource = parsed.String()
				} else if base, baseErr := url.Parse(currentSource); baseErr == nil && base.IsAbs() && (strings.EqualFold(base.Scheme, "http") || strings.EqualFold(base.Scheme, "https")) {
					targetSource = base.ResolveReference(parsed).String()
				} else if currentDirectory != "" {
					if absolute, err := filepath.Abs(filepath.Join(currentDirectory, filepath.FromSlash(name))); err == nil {
						if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
							absolute = resolved
						}
						targetSource = absolute
						targetDirectory = filepath.Dir(absolute)
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
					Related: []ir.SourceLocation{{Source: currentDisplay, Pointer: currentPointer + "/$ref"}},
				}
				identity := targetSource + targetPointer
				if !visiting[identity] {
					visiting[identity] = true
					referenced := remoteSources[targetSource]
					if targetDirectory != "" {
						referenced, _ = os.ReadFile(targetSource)
					}
					if len(referenced) != 0 {
						var referencedValue any
						if yaml.Unmarshal(referenced, &referencedValue) == nil {
							targetValue := referencedValue
							found := targetPointer == "#"
							if !found {
								targetValue, found = resolveLocalReference(referencedValue, targetPointer)
							}
							if found {
								visit(targetValue, path, targetSource, targetSource, targetDirectory, targetPointer)
							}
						}
					}
					delete(visiting, identity)
				}
			}
			for key, child := range typed {
				if key == "$ref" || referenceTraversalOpaque(path, key, child) {
					continue
				}
				token := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
				visit(child, append(path, key), currentSource, currentDisplay, currentDirectory, currentPointer+"/"+token)
			}
		case []any:
			for index, child := range typed {
				token := itoa(index)
				visit(child, append(path, token), currentSource, currentDisplay, currentDirectory, currentPointer+"/"+token)
			}
		}
	}
	visit(value, nil, resolutionSource, displaySource, directory, "#")
	return result
}

func propagateReferenceProvenance(raw map[string]any, references, result map[string]ir.Provenance) {
	pointers := make([]string, 0, len(references))
	for pointer := range references {
		pointers = append(pointers, pointer)
	}
	sort.Slice(pointers, func(left, right int) bool {
		if len(pointers[left]) == len(pointers[right]) {
			return pointers[left] < pointers[right]
		}
		return len(pointers[left]) < len(pointers[right])
	})
	type redirect struct {
		from string
		to   string
	}
	var redirects []redirect
	for _, pointer := range pointers {
		resolvedPointer := pointer
		value, found := resolveLocalReference(raw, resolvedPointer)
		if !found {
			for index := len(redirects) - 1; index >= 0; index-- {
				candidate := redirects[index]
				if pointer == candidate.from || strings.HasPrefix(pointer, candidate.from+"/") {
					resolvedPointer = candidate.to + strings.TrimPrefix(pointer, candidate.from)
					value, found = resolveLocalReference(raw, resolvedPointer)
					if found {
						break
					}
				}
			}
		}
		if !found {
			continue
		}
		provenance := references[pointer]
		if object, ok := value.(map[string]any); ok {
			if local, _ := object["$ref"].(string); strings.HasPrefix(local, "#/") {
				if target, targetFound := resolveLocalReference(raw, local); targetFound {
					redirects = append(redirects, redirect{from: pointer, to: local})
					visitReferencedProvenance(target, local, provenance.Primary.Pointer, provenance, result)
					continue
				}
			}
		}
		visitReferencedProvenance(value, resolvedPointer, provenance.Primary.Pointer, provenance, result)
	}
}

func visitReferencedProvenance(value any, bundledPointer, sourcePointer string, provenance ir.Provenance, result map[string]ir.Provenance) {
	result[bundledPointer] = ir.Provenance{
		Primary: ir.SourceLocation{Source: provenance.Primary.Source, Pointer: sourcePointer},
		Related: append([]ir.SourceLocation(nil), provenance.Related...),
	}
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			token := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
			visitReferencedProvenance(typed[key], bundledPointer+"/"+token, sourcePointer+"/"+token, provenance, result)
		}
	case []any:
		for index, child := range typed {
			token := itoa(index)
			visitReferencedProvenance(child, bundledPointer+"/"+token, sourcePointer+"/"+token, provenance, result)
		}
	}
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
