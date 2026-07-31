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
	result, failures := generatedStreamsDiagnostics(document, manifest)
	if len(failures) != 0 {
		return nil, failures[0]
	}
	return result, nil
}

func generatedStreamsDiagnostics(document *ir.Document, manifest Manifest) ([]generatedStream, []error) {
	visible := map[string]bool{}
	for _, operation := range manifest.Operations {
		if operation.Visibility != "hidden" {
			visible[manifestRouteKey(operation)] = true
		}
	}
	var result []generatedStream
	var failures []error
	for _, operation := range document.Operations {
		if !visible[operationRouteKey(operation)] {
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
				failures = append(failures, fmt.Errorf("streaming response %s %s: %w", operationLabel(operation), status, err))
				continue
			}
			content, _ := response["content"].(map[string]any)
			for _, mediaType := range sortedAnyKeys(content) {
				media, _ := content[mediaType].(map[string]any)
				media, err = resolveMediaTypeObject(document, media)
				if err != nil {
					failures = append(failures, fmt.Errorf("streaming response %s %s: %w", operationLabel(operation), mediaType, err))
					continue
				}
				if !isStreamingMediaType(mediaType, media) && media["itemSchema"] == nil {
					continue
				}
				itemSchema, exists := media["itemSchema"]
				if !exists {
					failures = append(failures, fmt.Errorf("streaming response %s %s has no itemSchema", operationLabel(operation), mediaType))
					continue
				}
				itemType, err := schemaTypeForScope(document, itemSchema, projectionOutput, typeRenderContract)
				if err != nil {
					failures = append(failures, fmt.Errorf("streaming response %s %s item schema: %w", operationLabel(operation), mediaType, err))
					continue
				}
				types = append(types, itemType)
			}
		}
		if len(types) != 0 {
			result = append(result, generatedStream{Operation: operation, ItemType: stringsJoinUnique(types, " | ")})
		}
	}
	sort.Slice(result, func(left, right int) bool {
		return operationRouteKey(result[left].Operation) < operationRouteKey(result[right].Operation)
	})
	return result, failures
}

func emitStreamInterface(output *bytes.Buffer, document *ir.Document, streams []generatedStream) error {
	if len(streams) == 0 {
		return nil
	}
	output.WriteString("  /** Lazy typed response streams keyed by OpenAPI operation ID. */\n")
	output.WriteString("  readonly $streams: {\n")
	for _, stream := range streams {
		if stream.Operation.OperationID == "" {
			continue
		}
		fmt.Fprintf(output, "    readonly %s: StreamCall<%s>\n", quoteTS(stream.Operation.OperationID), quoteTS(operationRouteKey(stream.Operation)))
	}
	output.WriteString("  }\n")
	return nil
}

func emitStreamValues(output *bytes.Buffer, document *ir.Document, streams []generatedStream) error {
	for _, stream := range streams {
		definition, err := operationDefinition(document, stream.Operation, ManifestOperation{RouteKey: operationRouteKey(stream.Operation), OperationID: stream.Operation.OperationID, Method: stream.Operation.Method, Path: stream.Operation.Path, Envelope: stream.Operation.Envelope})
		if err != nil {
			return err
		}
		inputs, err := operationInputTypes(document, stream.Operation)
		if err != nil {
			return err
		}
		inputRequired, err := operationInputRequired(document, stream.Operation, inputs, false)
		if err != nil {
			return err
		}
		inputType := operationSlotType(operationRouteKey(stream.Operation), "input")
		optionsType := operationSlotType(operationRouteKey(stream.Operation), "options")
		variable := stablePrivateIdentifier("stream-value", operationRouteKey(stream.Operation))
		functionType, err := streamFunctionType(document, stream)
		if err != nil {
			return err
		}
		hasInput := len(inputs) > 0
		inputOptional := hasInput && !inputRequired
		fmt.Fprintf(output, "  const %s = bindStreamOperation<%s, %s, %s>(request, %s, %t, %t) as %s\n", variable, inputType, stream.ItemType, optionsType, definition, hasInput, inputOptional, functionType)
	}
	return nil
}

func emitStreamReturnValue(output *bytes.Buffer, streams []generatedStream) error {
	if len(streams) == 0 {
		return nil
	}
	values := make([]runtimeProperty, 0, len(streams))
	for _, stream := range streams {
		if stream.Operation.OperationID == "" {
			continue
		}
		values = append(values, runtimeProperty{
			key:   stream.Operation.OperationID,
			value: stablePrivateIdentifier("stream-value", operationRouteKey(stream.Operation)),
		})
	}
	fmt.Fprintf(output, "    $streams: %s as unknown as Client[\"$streams\"],\n", runtimeObjectExpression(values))
	return nil
}

func streamForRoute(streams []generatedStream, routeKey string) (generatedStream, bool) {
	for _, stream := range streams {
		if operationRouteKey(stream.Operation) == routeKey {
			return stream, true
		}
	}
	return generatedStream{}, false
}

func streamFunctionType(document *ir.Document, stream generatedStream) (string, error) {
	inputs, err := operationInputTypes(document, stream.Operation)
	if err != nil {
		return "", err
	}
	inputRequired, err := operationInputRequired(document, stream.Operation, inputs, false)
	if err != nil {
		return "", err
	}
	optionsType := operationSlotType(operationRouteKey(stream.Operation), "options")
	optionMarker := "?"
	if operationRequiresOptions(stream.Operation) {
		optionMarker = ""
	}
	if len(inputs) == 0 {
		return "(options" + optionMarker + ": " + optionsType + ") => AsyncIterable<" + stream.ItemType + ">", nil
	}
	inputType := operationSlotType(operationRouteKey(stream.Operation), "input")
	if !inputRequired {
		optionsOnly := "(options" + optionMarker + ": " + optionsType + ") => AsyncIterable<" + stream.ItemType + ">"
		inputMarker := ""
		if optionMarker == "?" {
			inputMarker = "?"
		} else {
			inputType += " | undefined"
		}
		inputCall := "(input" + inputMarker + ": " + inputType + ", options" + optionMarker + ": " + optionsType + ") => AsyncIterable<" + stream.ItemType + ">"
		return "(" + optionsOnly + ") & (" + inputCall + ")", nil
	}
	inputMarker := ""
	return "(input" + inputMarker + ": " + inputType + ", options" + optionMarker + ": " + optionsType + ") => AsyncIterable<" + stream.ItemType + ">", nil
}

func operationRequiresOptions(ir.Operation) bool {
	return false
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
