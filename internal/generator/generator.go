// Package generator defines the language-neutral boundary between the OpenAPI compiler and SDK targets.
package generator

import (
	"fmt"
	"sort"

	"github.com/connextable/openapi-sdkgen/internal/compiler/ir"
)

// Artifact is one file emitted by a target, relative to the requested output directory.
type Artifact struct {
	Path string
	Data []byte
}

// Options are shared target options selected by the CLI.
type Options struct {
	PackageName string
}

// Target emits a client package from the language-neutral OpenAPI IR.
type Target interface {
	Name() string
	Generate(*ir.Document, Options) ([]Artifact, error)
}

// Registry holds the built-in SDK targets available to the CLI.
type Registry struct {
	targets map[string]Target
}

// NewRegistry validates and registers built-in targets.
func NewRegistry(targets ...Target) (*Registry, error) {
	registry := &Registry{targets: make(map[string]Target, len(targets))}
	for _, target := range targets {
		if target == nil || target.Name() == "" {
			return nil, fmt.Errorf("SDK target name is required")
		}
		if _, exists := registry.targets[target.Name()]; exists {
			return nil, fmt.Errorf("SDK target %q is registered more than once", target.Name())
		}
		registry.targets[target.Name()] = target
	}
	return registry, nil
}

// Lookup returns a registered target by its CLI name.
func (r *Registry) Lookup(name string) (Target, error) {
	if target, exists := r.targets[name]; exists {
		return target, nil
	}
	return nil, fmt.Errorf("unsupported SDK target %q (available: %s)", name, joinNames(r.Names()))
}

// Names returns the registered target names in stable order.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.targets))
	for name := range r.targets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func joinNames(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	result := names[0]
	for _, name := range names[1:] {
		result += ", " + name
	}
	return result
}
