package ir

type Document struct {
	Title              string
	ContractVersion    string
	OpenAPIVersion     string
	OpenAPIVersionLine string
	Servers            []Server
	Operations         []Operation
	ComponentSchemas   map[string]map[string]any
	// Schemas is the target-neutral schema registry. Unlike ComponentSchemas it
	// retains boolean schemas and records the dialect/resource identity needed
	// for JSON Schema reference resolution.
	Schemas map[string]Schema
	Raw     map[string]any
}

// Schema is a normalized schema resource. Value remains lossless so target
// lowerers can preserve every version-specific JSON Schema keyword while the
// compiler owns resource identity and dialect selection in one place.
type Schema struct {
	Name        string
	Pointer     string
	ResourceURI string
	Dialect     string
	Value       any
}

type Server struct {
	URL         string
	Description string
}

type Operation struct {
	OperationID        string
	Method             string
	Path               string
	Summary            string
	Description        string
	Tags               []string
	Visibility         string
	Envelope           string
	Concurrency        string
	Idempotency        string
	Pagination         string
	PathParameterOrder []string
	PathItemRaw        map[string]any
	Raw                map[string]any
}
