package sdkgen

import (
	"encoding/json"
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
	return compileFile(path, false)
}

func CompileProjectFile(path string) (*ir.Document, error) {
	return compileFile(path, true)
}

func compileFile(path string, project bool) (*ir.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read OpenAPI document: %w", err)
	}
	if project {
		if err := rejectProjectExternalReferences(data); err != nil {
			return nil, err
		}
	}
	bundled, err := bundler.BundleBytesComposed(data, &datamodel.DocumentConfiguration{
		BasePath:              filepath.Dir(path),
		SpecFilePath:          path,
		AllowFileReferences:   true,
		AllowRemoteReferences: false,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("resolve OpenAPI references: %w", err)
	}
	var value any
	if err := yaml.Unmarshal(bundled, &value); err != nil {
		return nil, fmt.Errorf("decode bundled OpenAPI document: %w", err)
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("normalize bundled OpenAPI document: %w", err)
	}
	return compile(normalized, project)
}

func rejectProjectExternalReferences(data []byte) error {
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
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
	document, err := openapidoc.Read(data)
	if err != nil {
		return nil, err
	}
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
