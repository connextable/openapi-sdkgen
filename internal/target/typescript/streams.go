package typescript

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
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
				itemType, err := schemaTypeForScope(document, itemSchema, projectionOutput, typeRenderContract)
				if err != nil {
					return nil, err
				}
				types = append(types, itemType)
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
		inputs, err := operationInputTypes(document, stream.Operation)
		if err != nil {
			return err
		}
		inputType := operationSlotType(stream.Operation.OperationID, "input")
		optionsType := operationSlotType(stream.Operation.OperationID, "options")
		optionMarker := "?"
		if operationRequiresOptions(stream.Operation) {
			optionMarker = ""
		}
		if len(inputs) == 0 {
			fmt.Fprintf(output, "    readonly %s: (options%s: %s) => AsyncIterable<%s>\n", quoteTS(stream.Operation.OperationID), optionMarker, optionsType, stream.ItemType)
		} else {
			fmt.Fprintf(output, "    readonly %s: (input: %s, options%s: %s) => AsyncIterable<%s>\n", quoteTS(stream.Operation.OperationID), inputType, optionMarker, optionsType, stream.ItemType)
		}
	}
	output.WriteString("  }\n")
	return nil
}

func emitStreamValues(output *bytes.Buffer, document *ir.Document, streams []generatedStream) error {
	for _, stream := range streams {
		definition, err := operationDefinition(document, stream.Operation, ManifestOperation{OperationID: stream.Operation.OperationID, Method: stream.Operation.Method, Path: stream.Operation.Path, Envelope: stream.Operation.Envelope})
		if err != nil {
			return err
		}
		inputs, err := operationInputTypes(document, stream.Operation)
		if err != nil {
			return err
		}
		inputType := operationSlotType(stream.Operation.OperationID, "input")
		optionsType := operationSlotType(stream.Operation.OperationID, "options")
		optionMarker := "?"
		if operationRequiresOptions(stream.Operation) {
			optionMarker = ""
		}
		variable := stablePrivateIdentifier("stream-value", stream.Operation.OperationID)
		if len(inputs) == 0 {
			fmt.Fprintf(output, "  const %s = (options%s: %s): AsyncIterable<%s> => request.stream<%s>(%s, undefined, options)\n", variable, optionMarker, optionsType, stream.ItemType, stream.ItemType, definition)
		} else {
			fmt.Fprintf(output, "  const %s = (input: %s, options%s: %s): AsyncIterable<%s> => request.stream<%s>(%s, input, options)\n", variable, inputType, optionMarker, optionsType, stream.ItemType, stream.ItemType, definition)
		}
	}
	return nil
}

func operationRequiresOptions(operation ir.Operation) bool {
	return operation.Idempotency == "required" || operation.Concurrency == "required"
}

func emitStreamReturnValue(output *bytes.Buffer, streams []generatedStream) error {
	if len(streams) == 0 {
		return nil
	}
	values := make([]runtimeProperty, 0, len(streams))
	for _, stream := range streams {
		values = append(values, runtimeProperty{
			key:   stream.Operation.OperationID,
			value: stablePrivateIdentifier("stream-value", stream.Operation.OperationID),
		})
	}
	fmt.Fprintf(output, "    $streams: %s as unknown as Client[\"$streams\"],\n", runtimeObjectExpression(values))
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
