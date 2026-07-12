package openapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"

	"github.com/pb33f/libopenapi"
	"go.yaml.in/yaml/v4"
)

type VersionLine string

const (
	Version30 VersionLine = "3.0"
	Version31 VersionLine = "3.1"
	Version32 VersionLine = "3.2"
)

var openAPIVersionPattern = regexp.MustCompile(`^3\.(0|1|2)\.(0|[1-9][0-9]*)(?:-(?:(?:0|[1-9][0-9]*)|(?:[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))(?:\.(?:(?:0|[1-9][0-9]*)|(?:[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)))*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

type Document struct {
	Raw     map[string]any
	Version VersionLine
}

func Read(data []byte) (*Document, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("OpenAPI document is empty")
	}
	var raw map[string]any
	yamlSource := false
	trimmed := bytes.TrimSpace(data)
	if bytes.HasPrefix(trimmed, []byte("{")) {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&raw); err != nil {
			// YAML flow mappings also start with `{`. Prefer strict JSON when it
			// parses, but fall back to YAML so valid OpenAPI YAML is not rejected
			// solely by its first byte.
			if yamlErr := decodeOpenAPIYAML(data, &raw); yamlErr != nil {
				return nil, fmt.Errorf("decode OpenAPI JSON: %w", err)
			}
			yamlSource = true
			goto decoded
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			if yamlErr := decodeOpenAPIYAML(data, &raw); yamlErr == nil {
				yamlSource = true
				goto decoded
			}
			if err == nil {
				return nil, errors.New("OpenAPI JSON contains trailing data")
			}
			return nil, fmt.Errorf("decode trailing OpenAPI JSON: %w", err)
		}
	} else {
		if err := decodeOpenAPIYAML(data, &raw); err != nil {
			return nil, fmt.Errorf("decode OpenAPI YAML: %w", err)
		}
		yamlSource = true
	}

decoded:
	version, _ := raw["openapi"].(string)
	versionLine, err := DetectVersionLine(version)
	if err != nil {
		return nil, err
	}
	if err := validateVersionSpecificFeatures(raw, versionLine); err != nil {
		return nil, err
	}

	parseData := data
	if yamlSource {
		parseData, err = json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("normalize OpenAPI YAML: %w", err)
		}
	}
	document, err := libopenapi.NewDocument(parseData)
	if err != nil {
		return nil, fmt.Errorf("parse OpenAPI document: %w", err)
	}
	if _, err := document.BuildV3Model(); err != nil {
		return nil, fmt.Errorf("build OpenAPI 3 model: %w", err)
	}
	return &Document{Raw: raw, Version: versionLine}, nil
}

func decodeOpenAPIYAML(data []byte, raw *map[string]any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(raw); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("OpenAPI YAML contains multiple documents")
		}
		return err
	}
	return nil
}

// DetectVersionLine classifies an OpenAPI semantic version that this compiler
// can parse. Patch releases and valid SemVer pre-release/build metadata within
// supported minor lines share the same parser adapter; a newer minor line must
// be explicitly added here.
func DetectVersionLine(version string) (VersionLine, error) {
	matches := openAPIVersionPattern.FindStringSubmatch(version)
	if len(matches) < 2 {
		return "", fmt.Errorf("unsupported OpenAPI version %q: supported versions are 3.0.x, 3.1.x, and 3.2.x", version)
	}

	switch matches[1] {
	case "0":
		return Version30, nil
	case "1":
		return Version31, nil
	case "2":
		return Version32, nil
	default:
		panic("unreachable OpenAPI version line")
	}
}
