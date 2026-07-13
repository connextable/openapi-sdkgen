package sdkgen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"
)

const schemaExtensionProtocolVersion = 1

type schemaExtensionManifest struct {
	Version    int                         `json:"version"`
	Extensions []schemaExtensionDefinition `json:"extensions"`
}

type schemaExtensionDefinition struct {
	Vocabularies []string `json:"vocabularies"`
	Command      string   `json:"command"`
	Args         []string `json:"args"`
	SHA256       string   `json:"sha256"`
}

type schemaExtension struct {
	definition schemaExtensionDefinition
	path       string
	digest     string
}

// lowerSchemaExtensions executes explicitly registered custom-vocabulary
// compilers before the document reaches the OpenAPI parser. The extension
// returns a replacement JSON Schema fragment, so generated code contains only
// deterministic lowered semantics and never invokes the extension at runtime.
func lowerSchemaExtensions(data []byte, options CompileOptions, lock *referenceLock) ([]byte, error) {
	var document any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("inspect schema vocabularies: %w", err)
	}
	required := requiredCustomVocabularies(document)
	if len(required) == 0 {
		return data, nil
	}
	if lock == nil {
		return nil, errors.New("required custom JSON Schema vocabulary needs --schema-extension and --update-ref-lock")
	}
	extensions, err := loadSchemaExtensions(options.SchemaExtensionManifests, lock, options.UpdateRefLock)
	if err != nil {
		return nil, err
	}
	vocabularies := sortedVocabularyURIs(required)
	for _, vocabulary := range vocabularies {
		extension, ok := extensions[vocabulary]
		if !ok {
			return nil, fmt.Errorf("required custom JSON Schema vocabulary %q has no registered --schema-extension", vocabulary)
		}
		if err := extension.describe(); err != nil {
			return nil, fmt.Errorf("schema extension for vocabulary %q: %w", vocabulary, err)
		}
	}
	lowered, err := lowerSchemaExtensionValue(document, "", extensions)
	if err != nil {
		return nil, err
	}
	if remaining := requiredCustomVocabularies(lowered); len(remaining) != 0 {
		return nil, fmt.Errorf("schema extension output still requires custom JSON Schema vocabulary %q", sortedVocabularyURIs(remaining)[0])
	}
	encoded, err := json.Marshal(lowered)
	if err != nil {
		return nil, fmt.Errorf("encode lowered schema extensions: %w", err)
	}
	return encoded, nil
}

// validateSchemaExtensions remains the narrow integrity check used by callers
// that only need to validate an extension configuration. Compilation uses
// lowerSchemaExtensions above and consumes its returned document.
func validateSchemaExtensions(data []byte, options CompileOptions, lock *referenceLock) error {
	var document any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("inspect schema vocabularies: %w", err)
	}
	required := requiredCustomVocabularies(document)
	if len(required) == 0 {
		return nil
	}
	if lock == nil {
		return errors.New("required custom JSON Schema vocabulary needs --schema-extension and --update-ref-lock")
	}
	extensions, err := loadSchemaExtensions(options.SchemaExtensionManifests, lock, options.UpdateRefLock)
	if err != nil {
		return err
	}
	for _, vocabulary := range sortedVocabularyURIs(required) {
		extension, ok := extensions[vocabulary]
		if !ok {
			return fmt.Errorf("required custom JSON Schema vocabulary %q has no registered --schema-extension", vocabulary)
		}
		if err := extension.describe(); err != nil {
			return fmt.Errorf("schema extension for vocabulary %q: %w", vocabulary, err)
		}
	}
	return nil
}

func lowerSchemaExtensionValue(value any, pointer string, extensions map[string]schemaExtension) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		current := any(typed)
		if required := requiredCustomVocabulariesAt(typed); len(required) != 0 {
			for _, vocabulary := range sortedVocabularyURIs(required) {
				extension, ok := extensions[vocabulary]
				if !ok {
					return nil, fmt.Errorf("required custom JSON Schema vocabulary %q at #%s has no registered --schema-extension", vocabulary, pointer)
				}
				var err error
				current, err = extension.lower(vocabulary, pointer, current)
				if err != nil {
					return nil, fmt.Errorf("schema extension for vocabulary %q at #%s: %w", vocabulary, pointer, err)
				}
			}
		}
		object, ok := current.(map[string]any)
		if !ok {
			if boolean, ok := current.(bool); ok {
				return boolean, nil
			}
			return nil, fmt.Errorf("schema extension output at #%s must be a JSON Schema object or boolean", pointer)
		}
		result := make(map[string]any, len(object))
		for key, child := range object {
			if schemaReferenceLiteralKey(key) {
				result[key] = child
				continue
			}
			normalized, err := lowerSchemaExtensionValue(child, appendSchemaPointer(pointer, key), extensions)
			if err != nil {
				return nil, err
			}
			result[key] = normalized
		}
		return result, nil
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			normalized, err := lowerSchemaExtensionValue(child, appendSchemaPointer(pointer, fmt.Sprint(index)), extensions)
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

func requiredCustomVocabulariesAt(value map[string]any) map[string]struct{} {
	required := map[string]struct{}{}
	vocabularies, _ := value["$vocabulary"].(map[string]any)
	for uri, enabled := range vocabularies {
		if requiredFlag, ok := enabled.(bool); ok && requiredFlag && !isStandardVocabulary(uri) {
			required[uri] = struct{}{}
		}
	}
	return required
}

func sortedVocabularyURIs(vocabularies map[string]struct{}) []string {
	result := make([]string, 0, len(vocabularies))
	for vocabulary := range vocabularies {
		result = append(result, vocabulary)
	}
	sort.Strings(result)
	return result
}

func requiredCustomVocabularies(value any) map[string]struct{} {
	required := map[string]struct{}{}
	var visit func(any)
	visit = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			if vocabularies, ok := typed["$vocabulary"].(map[string]any); ok {
				for uri, enabled := range vocabularies {
					if requiredFlag, ok := enabled.(bool); ok && requiredFlag && !isStandardVocabulary(uri) {
						required[uri] = struct{}{}
					}
				}
			}
			for _, child := range typed {
				visit(child)
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		}
	}
	visit(value)
	return required
}

func isStandardVocabulary(uri string) bool {
	return strings.HasPrefix(uri, "https://json-schema.org/draft/2020-12/vocab/") || uri == "https://spec.openapis.org/oas/3.1/dialect/base"
}

func loadSchemaExtensions(paths []string, lock *referenceLock, update bool) (map[string]schemaExtension, error) {
	result := map[string]schemaExtension{}
	for _, manifestPath := range paths {
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("read schema extension manifest %s: %w", manifestPath, err)
		}
		var manifest schemaExtensionManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("decode schema extension manifest %s: %w", manifestPath, err)
		}
		if manifest.Version != schemaExtensionProtocolVersion {
			return nil, fmt.Errorf("schema extension manifest %s has unsupported version %d", manifestPath, manifest.Version)
		}
		for _, definition := range manifest.Extensions {
			extension, err := prepareSchemaExtension(manifestPath, definition, lock, update)
			if err != nil {
				return nil, err
			}
			for _, vocabulary := range definition.Vocabularies {
				if vocabulary == "" {
					return nil, fmt.Errorf("schema extension manifest %s declares an empty vocabulary URI", manifestPath)
				}
				if _, exists := result[vocabulary]; exists {
					return nil, fmt.Errorf("multiple schema extensions declare vocabulary %q", vocabulary)
				}
				result[vocabulary] = extension
			}
		}
	}
	return result, nil
}

func prepareSchemaExtension(manifestPath string, definition schemaExtensionDefinition, lock *referenceLock, update bool) (schemaExtension, error) {
	if definition.Command == "" || len(definition.Vocabularies) == 0 || definition.SHA256 == "" {
		return schemaExtension{}, fmt.Errorf("schema extension manifest %s requires command, vocabularies, and sha256", manifestPath)
	}
	path := definition.Command
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(manifestPath), path)
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return schemaExtension{}, fmt.Errorf("resolve schema extension executable %s: %w", definition.Command, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return schemaExtension{}, fmt.Errorf("inspect schema extension executable %s: %w", path, err)
	}
	if info.Mode()&0o111 == 0 || !info.Mode().IsRegular() {
		return schemaExtension{}, fmt.Errorf("schema extension executable %s is not an executable regular file", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return schemaExtension{}, fmt.Errorf("read schema extension executable %s: %w", path, err)
	}
	digestBytes := sha256.Sum256(data)
	digest := hex.EncodeToString(digestBytes[:])
	if !strings.EqualFold(definition.SHA256, digest) {
		return schemaExtension{}, fmt.Errorf("schema extension executable %s digest does not match manifest", path)
	}
	if previous, exists := lock.Extensions[path]; exists && previous != digest && !update {
		return schemaExtension{}, fmt.Errorf("schema extension executable %s digest changed; run with --update-ref-lock to accept it", path)
	}
	if !update {
		if _, exists := lock.Extensions[path]; !exists {
			return schemaExtension{}, fmt.Errorf("schema extension executable %s is missing from the reference lock; run with --update-ref-lock first", path)
		}
	} else {
		lock.Extensions[path] = digest
	}
	return schemaExtension{definition: definition, path: path, digest: digest}, nil
}

func (extension schemaExtension) describe() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request := []byte(`{"jsonrpc":"2.0","id":1,"method":"describe","params":{"protocolVersion":1}}` + "\n")
	command := exec.CommandContext(ctx, extension.path, extension.definition.Args...)
	command.Stdin = bytes.NewReader(request)
	var output bytes.Buffer
	command.Stdout = &limitedWriter{writer: &output, max: 1 << 20}
	var stderr bytes.Buffer
	command.Stderr = &limitedWriter{writer: &stderr, max: 16 << 10}
	if err := command.Run(); err != nil {
		return fmt.Errorf("run describe: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var response struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			Vocabularies []string `json:"vocabularies"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		return fmt.Errorf("decode describe response: %w", err)
	}
	if response.JSONRPC != "2.0" || response.ID != 1 {
		return errors.New("describe response does not match JSON-RPC request")
	}
	for _, vocabulary := range response.Result.Vocabularies {
		if vocabulary == "" {
			return errors.New("describe response contains an empty vocabulary URI")
		}
	}
	declared := make(map[string]struct{}, len(response.Result.Vocabularies))
	for _, vocabulary := range response.Result.Vocabularies {
		declared[vocabulary] = struct{}{}
	}
	for _, vocabulary := range extension.definition.Vocabularies {
		if _, ok := declared[vocabulary]; !ok {
			return fmt.Errorf("describe response does not declare manifest vocabulary %q", vocabulary)
		}
	}
	return nil
}

func (extension schemaExtension) lower(vocabulary, pointer string, schema any) (any, error) {
	requestBody, err := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Method  string `json:"method"`
		Params  struct {
			ProtocolVersion int    `json:"protocolVersion"`
			Target          string `json:"target"`
			Vocabulary      string `json:"vocabulary"`
			Location        string `json:"location"`
			Schema          any    `json:"schema"`
		} `json:"params"`
	}{
		JSONRPC: "2.0", ID: 1, Method: "lower",
		Params: struct {
			ProtocolVersion int    `json:"protocolVersion"`
			Target          string `json:"target"`
			Vocabulary      string `json:"vocabulary"`
			Location        string `json:"location"`
			Schema          any    `json:"schema"`
		}{ProtocolVersion: schemaExtensionProtocolVersion, Target: "typescript", Vocabulary: vocabulary, Location: "#" + pointer, Schema: schema},
	})
	if err != nil {
		return nil, fmt.Errorf("encode lower request: %w", err)
	}
	output, err := extension.call(requestBody)
	if err != nil {
		return nil, fmt.Errorf("run lower: %w", err)
	}
	var response struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			Schema json.RawMessage `json:"schema"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("decode lower response: %w", err)
	}
	if response.JSONRPC != "2.0" || response.ID != 1 {
		return nil, errors.New("lower response does not match JSON-RPC request")
	}
	if response.Error != nil {
		return nil, fmt.Errorf("extension rejected schema: %s", response.Error.Message)
	}
	if len(response.Result.Schema) == 0 {
		return nil, errors.New("lower response has no schema")
	}
	var lowered any
	if err := json.Unmarshal(response.Result.Schema, &lowered); err != nil {
		return nil, fmt.Errorf("decode lowered schema: %w", err)
	}
	if _, object := lowered.(map[string]any); !object {
		if _, boolean := lowered.(bool); !boolean {
			return nil, errors.New("lower response schema must be an object or boolean")
		}
	}
	return lowered, nil
}

func (extension schemaExtension) call(request []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, extension.path, extension.definition.Args...)
	command.Stdin = bytes.NewReader(append(request, '\n'))
	var output bytes.Buffer
	command.Stdout = &limitedWriter{writer: &output, max: 1 << 20}
	var stderr bytes.Buffer
	command.Stderr = &limitedWriter{writer: &stderr, max: 16 << 10}
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return output.Bytes(), nil
}

type limitedWriter struct {
	writer io.Writer
	max    int
	wrote  int
}

func (writer *limitedWriter) Write(data []byte) (int, error) {
	if writer.wrote+len(data) > writer.max {
		return 0, errors.New("extension output exceeds size limit")
	}
	n, err := writer.writer.Write(data)
	writer.wrote += n
	return n, err
}
