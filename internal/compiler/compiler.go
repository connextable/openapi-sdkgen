package sdkgen

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pb33f/libopenapi/bundler"
	"github.com/pb33f/libopenapi/datamodel"
	"go.yaml.in/yaml/v4"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	openapidoc "github.com/connextable/openapi-sdkgen/internal/compiler/openapi"
	"github.com/connextable/openapi-sdkgen/internal/compiler/validate"
)

func Compile(data []byte) (*ir.Document, error) {
	return compile(data, false)
}

func CompileProject(data []byte) (*ir.Document, error) {
	return compile(data, true)
}

func CompileFile(path string) (*ir.Document, error) {
	return CompileFileWithOptions(path, CompileOptions{})
}

// CompileFileWithOptions compiles an OpenAPI document using explicit opt-in
// reference and extension capabilities. It never fetches a remote reference
// unless RemoteRefAllowlist is populated.
func CompileFileWithOptions(path string, options CompileOptions) (*ir.Document, error) {
	return compileFile(path, false, options)
}

func CompileProjectFile(path string) (*ir.Document, error) {
	return compileFile(path, true, CompileOptions{})
}

func compileFile(path string, project bool, options CompileOptions) (*ir.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read OpenAPI document: %w", err)
	}
	if project && (len(options.RemoteRefAllowlist) != 0 || len(options.SchemaExtensionManifests) != 0 || options.UpdateRefLock || options.Offline || options.RefLockPath != "") {
		return nil, errors.New("project compilation does not support remote references or schema extensions")
	}
	if project {
		if err := rejectProjectExternalReferences(data); err != nil {
			return nil, err
		}
	} else if err := rejectEscapingFileReferencesWithRemote(path, filepath.Dir(path), len(options.RemoteRefAllowlist) != 0); err != nil {
		return nil, err
	}
	needsLock := len(options.RemoteRefAllowlist) != 0 || len(options.SchemaExtensionManifests) != 0
	lockPath := options.RefLockPath
	if lockPath == "" {
		lockPath = defaultReferenceLockPath(path)
	}
	var lock *referenceLock
	if needsLock {
		// A remote allowlist alone does not imply a network request. Defer a
		// missing-lock failure until a remote document or extension is actually
		// used, so offline documents stay reproducible without empty lockfiles.
		lock, err = loadReferenceLock(lockPath, true)
		if err != nil {
			return nil, err
		}
	}
	var remoteResolver *remoteReferenceResolver
	if len(options.RemoteRefAllowlist) != 0 {
		remoteResolver, err = newRemoteReferenceResolver(options, lock, filepath.Join(filepath.Dir(lockPath), ".openapi-sdkgen-cache"))
		if err != nil {
			return nil, err
		}
	}
	bundlerConfiguration := &datamodel.DocumentConfiguration{
		BasePath:              filepath.Dir(path),
		SpecFilePath:          path,
		AllowFileReferences:   true,
		AllowRemoteReferences: remoteResolver != nil,
	}
	if remoteResolver != nil {
		bundlerConfiguration.RemoteURLHandler = remoteResolver.handle
	}
	bundled, err := bundler.BundleBytesComposed(data, bundlerConfiguration, nil)
	if remoteResolver != nil {
		if remoteErr := remoteResolver.firstError(); remoteErr != nil {
			return nil, fmt.Errorf("resolve OpenAPI references: %w", remoteErr)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("resolve OpenAPI references: %w", err)
	}
	var value any
	if err := yaml.Unmarshal(bundled, &value); err != nil {
		return nil, fmt.Errorf("decode bundled OpenAPI document: %w", err)
	}
	var source any
	if err := yaml.Unmarshal(data, &source); err != nil {
		return nil, fmt.Errorf("decode source OpenAPI document: %w", err)
	}
	merged := mergeBundledDocument(source, value)
	merged, err = normalizeNestedSchemaReferences(merged)
	if err != nil {
		return nil, fmt.Errorf("normalize nested OpenAPI schema references: %w", err)
	}
	normalized, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("normalize bundled OpenAPI document: %w", err)
	}
	normalized, err = lowerSchemaExtensions(normalized, options, lock)
	if err != nil {
		return nil, err
	}
	document, err := compile(normalized, project)
	if err != nil {
		return nil, err
	}
	if needsLock && options.UpdateRefLock {
		if err := writeReferenceLock(lockPath, lock); err != nil {
			return nil, err
		}
	}
	return document, nil
}

// mergeBundledDocument keeps extensions and newly standardized OpenAPI fields
// that the bundler does not model yet, while retaining its resolved $ref output.
// This matters for an OpenAPI 3.2 document such as a Path Item's
// additionalOperations: it must reach the IR even when the CLI compiles a file.
func mergeBundledDocument(source, bundled any) any {
	sourceObject, sourceIsObject := source.(map[string]any)
	bundledObject, bundledIsObject := bundled.(map[string]any)
	if !sourceIsObject || !bundledIsObject {
		return bundled
	}
	result := make(map[string]any, len(sourceObject)+len(bundledObject))
	for key, value := range bundledObject {
		result[key] = value
	}
	for key, sourceValue := range sourceObject {
		if key == "$ref" {
			// The bundled value is the resolved reference. Restoring the source
			// value would undo external reference resolution.
			continue
		}
		if bundledValue, exists := bundledObject[key]; exists {
			result[key] = mergeBundledDocument(sourceValue, bundledValue)
			continue
		}
		result[key] = sourceValue
	}
	return result
}

func rejectEscapingFileReferences(path, root string) error {
	return rejectEscapingFileReferencesWithRemote(path, root, false)
}

func rejectEscapingFileReferencesWithRemote(path, root string, allowRemote bool) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve OpenAPI input directory: %w", err)
	}
	return inspectReferenceFile(path, resolvedRoot, make(map[string]bool), allowRemote)
}

func inspectReferenceFile(path, root string, visited map[string]bool, allowRemote bool) error {
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve OpenAPI reference file %s: %w", path, err)
	}
	if err := requireContainedPath(resolvedPath, root); err != nil {
		return err
	}
	if visited[resolvedPath] {
		return nil
	}
	visited[resolvedPath] = true
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return fmt.Errorf("read OpenAPI reference file %s: %w", resolvedPath, err)
	}
	var document any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("inspect OpenAPI references: %w", err)
	}
	var visit func(any) error
	visit = func(value any) error {
		switch typed := value.(type) {
		case map[string]any:
			if reference, _ := typed["$ref"].(string); reference != "" {
				target, err := resolveContainedReference(reference, filepath.Dir(resolvedPath), root, allowRemote)
				if err != nil {
					return err
				}
				if target != "" {
					if err := inspectReferenceFile(target, root, visited, allowRemote); err != nil {
						return err
					}
				}
			}
			for _, item := range typed {
				if err := visit(item); err != nil {
					return err
				}
			}
		case []any:
			for _, item := range typed {
				if err := visit(item); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return visit(document)
}

func resolveContainedReference(reference, directory, root string, allowRemote bool) (string, error) {
	file, _, _ := strings.Cut(reference, "#")
	if file == "" {
		return "", nil
	}
	if allowRemote && (strings.HasPrefix(file, "https://") || strings.HasPrefix(file, "http://")) {
		return "", nil
	}
	if filepath.IsAbs(file) || strings.Contains(file, "://") || strings.HasPrefix(file, "file:") {
		return "", fmt.Errorf("OpenAPI reference %q must stay inside the input directory", reference)
	}
	candidate := filepath.Join(directory, filepath.FromSlash(file))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve OpenAPI reference %q: %w", reference, err)
	}
	if err := requireContainedPath(resolved, root); err != nil {
		return "", fmt.Errorf("OpenAPI reference %q escapes the input directory: %w", reference, err)
	}
	return resolved, nil
}

func requireContainedPath(path, root string) error {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %s escapes input directory", path)
	}
	return nil
}

func rejectProjectExternalReferences(data []byte) error {
	var root any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("inspect project references: %w", err)
	}
	var visit func(any) error
	visit = func(value any) error {
		switch typed := value.(type) {
		case map[string]any:
			if reference, _ := typed["$ref"].(string); reference != "" && !strings.HasPrefix(reference, "#") {
				return fmt.Errorf("project OpenAPI artifacts must be self-contained; external reference %q is not allowed", reference)
			}
			for _, item := range typed {
				if err := visit(item); err != nil {
					return err
				}
			}
		case []any:
			for _, item := range typed {
				if err := visit(item); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return visit(root)
}

func compile(data []byte, project bool) (*ir.Document, error) {
	// libopenapi resolves `$ref` while it reads. JSON Schema anchors are valid
	// in OpenAPI 3.1/3.2 but are not component pointers, so normalize them
	// before the library's OpenAPI reference resolver sees the document.
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode OpenAPI document: %w", err)
	}
	normalized, err := normalizeNestedSchemaReferences(raw)
	if err != nil {
		return nil, fmt.Errorf("normalize nested OpenAPI schema references: %w", err)
	}
	normalizedDocument, ok := normalized.(map[string]any)
	if !ok {
		return nil, errors.New("normalized OpenAPI document must be an object")
	}
	normalizedData, err := json.Marshal(normalizedDocument)
	if err != nil {
		return nil, fmt.Errorf("encode normalized OpenAPI document: %w", err)
	}
	document, err := openapidoc.Read(normalizedData)
	if err != nil {
		return nil, err
	}
	// Retain normalization metadata not modeled by libopenapi.
	document.Raw = normalizedDocument
	model, err := ir.Build(document)
	if err != nil {
		return nil, err
	}
	if project {
		if err := validate.Project(model); err != nil {
			return nil, err
		}
	}
	return model, nil
}
