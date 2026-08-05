// Package playground exposes the in-memory generator pipeline used by the
// documentation playground.
package playground

import (
	"fmt"

	compiler "openapi-sdkgen/internal/compiler"
	"openapi-sdkgen/internal/diagnostic"
	"openapi-sdkgen/internal/generator"
	"openapi-sdkgen/internal/target/typescript"
)

// Artifact is one generated source file returned to the browser.
type Artifact struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Result is the complete browser-safe generation outcome.
type Result struct {
	Artifacts   []Artifact `json:"artifacts,omitempty"`
	Diagnostics string     `json:"diagnostics,omitempty"`
}

// Generate compiles one self-contained OpenAPI document without filesystem or
// network access and emits source for the selected target.
func Generate(data []byte, targetName string) (Result, error) {
	registry, err := generator.NewRegistry(typescript.Generator{})
	if err != nil {
		return Result{}, fmt.Errorf("configure playground targets: %w", err)
	}
	target, err := registry.Lookup(targetName)
	if err != nil {
		return Result{}, err
	}

	compiled, err := compiler.CompileResult(data)
	if err != nil {
		return Result{}, fmt.Errorf("compile OpenAPI document: %w", err)
	}
	prepared, err := generator.PrepareCompilation(target, compiled, generator.Options{})
	if err != nil {
		return Result{}, fmt.Errorf("prepare %s output: %w", target.Name(), err)
	}
	result := Result{}
	if len(prepared.Diagnostics) != 0 || len(prepared.SkippedPhases) != 0 {
		result.Diagnostics = diagnostic.RenderHuman(prepared.Diagnostics, prepared.SkippedPhases)
	}
	if diagnostic.HasErrors(prepared.Diagnostics) {
		return result, nil
	}

	artifacts, err := target.Emit(prepared.Plan)
	if err != nil {
		return Result{}, fmt.Errorf("emit %s output: %w", target.Name(), err)
	}
	result.Artifacts = make([]Artifact, len(artifacts))
	for index, artifact := range artifacts {
		result.Artifacts[index] = Artifact{Path: artifact.Path, Content: string(artifact.Data)}
	}
	return result, nil
}
