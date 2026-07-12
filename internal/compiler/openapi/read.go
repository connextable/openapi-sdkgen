package openapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/pb33f/libopenapi"
)

const SupportedVersion = "3.2.0"

type Document struct {
	Raw map[string]any
}

func Read(data []byte) (*Document, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("OpenAPI document is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var raw map[string]any
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode OpenAPI JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("OpenAPI JSON contains trailing data")
		}
		return nil, fmt.Errorf("decode trailing OpenAPI JSON: %w", err)
	}
	version, _ := raw["openapi"].(string)
	if version != SupportedVersion {
		return nil, fmt.Errorf("unsupported OpenAPI version %q: only %s is accepted", version, SupportedVersion)
	}

	document, err := libopenapi.NewDocument(data)
	if err != nil {
		return nil, fmt.Errorf("parse OpenAPI document: %w", err)
	}
	if _, err := document.BuildV3Model(); err != nil {
		return nil, fmt.Errorf("build OpenAPI 3.2 model: %w", err)
	}
	return &Document{Raw: raw}, nil
}
