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

// Addon names an optional, independently-addressable generated artifact set.
type Addon string

const (
	// AddonServer emits server-facing TypeScript artifacts alongside the default
	// client source tree.
	AddonServer Addon = "server"
)

// Options are shared target options selected by the CLI.
type Options struct {
	addons map[Addon]struct{}
}

// HasAddon reports whether an optional artifact set was selected.
func (o Options) HasAddon(addon Addon) bool {
	_, exists := o.addons[addon]
	return exists
}

// Addons returns selected add-ons in stable name order.
func (o Options) Addons() []Addon {
	addons := make([]Addon, 0, len(o.addons))
	for addon := range o.addons {
		addons = append(addons, addon)
	}
	sort.Slice(addons, func(i, j int) bool { return addons[i] < addons[j] })
	return addons
}

// AddonRegistry defines the optional artifact sets understood by the CLI.
type AddonRegistry struct {
	addons map[string]Addon
}

// NewAddonRegistry validates and registers supported add-ons.
func NewAddonRegistry(addons ...Addon) (*AddonRegistry, error) {
	registry := &AddonRegistry{addons: make(map[string]Addon, len(addons))}
	for _, addon := range addons {
		name := string(addon)
		if name == "" {
			return nil, fmt.Errorf("SDK add-on name is required")
		}
		if _, exists := registry.addons[name]; exists {
			return nil, fmt.Errorf("SDK add-on %q is registered more than once", name)
		}
		registry.addons[name] = addon
	}
	return registry, nil
}

// Resolve validates repeatable --with values and returns normalized options.
func (r *AddonRegistry) Resolve(values []string) (Options, error) {
	options := Options{addons: make(map[Addon]struct{}, len(values))}
	for _, value := range values {
		addon, exists := r.addons[value]
		if !exists {
			return Options{}, fmt.Errorf("unsupported SDK add-on %q (available: %s)", value, joinNames(r.Names()))
		}
		if _, duplicate := options.addons[addon]; duplicate {
			return Options{}, fmt.Errorf("SDK add-on %q was specified more than once", addon)
		}
		options.addons[addon] = struct{}{}
	}
	return options, nil
}

// Names returns registered add-on names in stable order.
func (r *AddonRegistry) Names() []string {
	names := make([]string, 0, len(r.addons))
	for name := range r.addons {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// AddonTarget declares which generated add-ons a target can emit.
type AddonTarget interface {
	SupportsAddon(Addon) bool
}

// ValidateTargetOptions rejects a selected add-on before document compilation
// when the chosen target cannot generate it.
func ValidateTargetOptions(target Target, options Options) error {
	supported, hasSupportDeclaration := target.(AddonTarget)
	for _, addon := range options.Addons() {
		if !hasSupportDeclaration || !supported.SupportsAddon(addon) {
			return fmt.Errorf("SDK target %q does not support add-on %q", target.Name(), addon)
		}
	}
	return nil
}

// Target emits client source files from the language-neutral OpenAPI IR.
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
