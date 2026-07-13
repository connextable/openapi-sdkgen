package typescript

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
	"github.com/connextable/openapi-sdkgen/internal/compiler/naming"
)

type generatedStream struct {
	Operation ir.Operation
	ItemType  string
}

func generatedStreams(document *ir.Document, manifest Manifest) ([]generatedStream, error) {
	visible := map[string]bool{}
	for _, operation := range manifest.Operations {
		if operation.Visibility != "hidden" {
			visible[operation.OperationID] = true
		}
	}
	var result []generatedStream
	for _, operation := range document.Operations {
		if !visible[operation.OperationID] {
			continue
		}
		responses, _ := operation.Raw["responses"].(map[string]any)
		var types []string
		for _, status := range sortedAnyKeys(responses) {
			if !isSuccessResponseStatus(status) {
				continue
			}
			response, _ := responses[status].(map[string]any)
			response, err := resolveComponentObject(document, response, "responses")
			if err != nil {
				return nil, err
			}
			content, _ := response["content"].(map[string]any)
			for _, mediaType := range sortedAnyKeys(content) {
				media, _ := content[mediaType].(map[string]any)
				media, err = resolveMediaTypeObject(document, media)
				if err != nil {
					return nil, err
				}
				if !isStreamingMediaType(mediaType, media) && media["itemSchema"] == nil {
					continue
				}
				itemSchema, exists := media["itemSchema"]
				if !exists {
					return nil, fmt.Errorf("streaming response %s %s has no itemSchema", operation.OperationID, mediaType)
				}
				itemType, err := schemaType(document, itemSchema, projectionOutput)
				if err != nil {
					return nil, err
				}
				types = append(types, qualifyClientType(document, itemType))
			}
		}
		if len(types) != 0 {
			result = append(result, generatedStream{Operation: operation, ItemType: stringsJoinUnique(types, " | ")})
		}
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].Operation.OperationID < result[right].Operation.OperationID
	})
	return result, nil
}

func emitStreamInterface(output *bytes.Buffer, document *ir.Document, streams []generatedStream) error {
	if len(streams) == 0 {
		return nil
	}
	output.WriteString("  /** Lazy typed response streams keyed by OpenAPI operation ID. */\n")
	output.WriteString("  readonly $streams: {\n")
	for _, stream := range streams {
		property, err := naming.Property(stream.Operation.OperationID)
		if err != nil {
			return err
		}
		inputs, err := operationInputTypes(document, stream.Operation)
		if err != nil {
			return err
		}
		name := operationTypeName(stream.Operation.OperationID)
		if len(inputs) == 0 {
			fmt.Fprintf(output, "    readonly %s: (options?: %sOptions) => AsyncIterable<%s>\n", property, name, stream.ItemType)
		} else {
			fmt.Fprintf(output, "    readonly %s: (input: %sInput, options?: %sOptions) => AsyncIterable<%s>\n", property, name, name, stream.ItemType)
		}
	}
	output.WriteString("  }\n")
	return nil
}

func emitStreamValues(output *bytes.Buffer, document *ir.Document, streams []generatedStream) error {
	for _, stream := range streams {
		property, err := naming.Property(stream.Operation.OperationID)
		if err != nil {
			return err
		}
		definition, err := operationDefinition(document, stream.Operation, ManifestOperation{OperationID: stream.Operation.OperationID, Method: stream.Operation.Method, Path: stream.Operation.Path, Envelope: stream.Operation.Envelope})
		if err != nil {
			return err
		}
		inputs, err := operationInputTypes(document, stream.Operation)
		if err != nil {
			return err
		}
		name := operationTypeName(stream.Operation.OperationID)
		variable := "stream" + name
		if len(inputs) == 0 {
			fmt.Fprintf(output, "  const %s = (options?: %sOptions): AsyncIterable<%s> => request.stream<%s>(%s, undefined, options)\n", variable, name, stream.ItemType, stream.ItemType, definition)
		} else {
			fmt.Fprintf(output, "  const %s = (input: %sInput, options?: %sOptions): AsyncIterable<%s> => request.stream<%s>(%s, input, options)\n", variable, name, name, stream.ItemType, stream.ItemType, definition)
		}
		_ = property
	}
	return nil
}

func emitStreamReturnValue(output *bytes.Buffer, streams []generatedStream) error {
	if len(streams) == 0 {
		return nil
	}
	output.WriteString("    $streams: {\n")
	for _, stream := range streams {
		property, err := naming.Property(stream.Operation.OperationID)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "      %s: stream%s,\n", property, operationTypeName(stream.Operation.OperationID))
	}
	output.WriteString("    },\n")
	return nil
}

func isStreamMediaType(mediaType string) bool {
	mediaType = strings.ToLower(mediaType)
	return strings.Contains(mediaType, "event-stream") || strings.Contains(mediaType, "json-seq") || strings.Contains(mediaType, "ndjson") || strings.Contains(mediaType, "jsonl")
}

func isStreamingMediaType(mediaType string, media map[string]any) bool {
	if isStreamMediaType(mediaType) {
		return true
	}
	_, hasItemSchema := media["itemSchema"]
	return hasItemSchema && strings.HasPrefix(strings.ToLower(mediaType), "multipart/")
}

func stringsJoinUnique(values []string, separator string) string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return strings.Join(result, separator)
}
