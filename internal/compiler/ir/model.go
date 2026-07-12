package ir

type Document struct {
	Title              string
	ContractVersion    string
	OpenAPIVersion     string
	OpenAPIVersionLine string
	Servers            []Server
	Operations         []Operation
	ComponentSchemas   map[string]map[string]any
	Raw                map[string]any
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
