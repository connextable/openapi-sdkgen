package naming

import "testing"

func TestIdentifierNormalization(t *testing.T) {
	for _, test := range []struct {
		input        string
		wantPublic   string
		wantProperty string
	}{
		{input: "operationId", wantPublic: "OperationID", wantProperty: "operationID"},
		{input: "base_url", wantPublic: "BaseURL", wantProperty: "baseURL"},
		{input: "oauth-token", wantPublic: "OAUTHToken", wantProperty: "oauthToken"},
		{input: "productID", wantPublic: "ProductID", wantProperty: "productID"},
		{input: "createdAtGte", wantPublic: "CreatedAtGTE", wantProperty: "createdAtGTE"},
		{input: "requestUri", wantPublic: "RequestURI", wantProperty: "requestURI"},
	} {
		t.Run(test.input, func(t *testing.T) {
			public, err := Public(test.input)
			if err != nil {
				t.Fatal(err)
			}
			property, err := Property(test.input)
			if err != nil {
				t.Fatal(err)
			}
			if public != test.wantPublic {
				t.Fatalf("public = %q, want %q", public, test.wantPublic)
			}
			if property != test.wantProperty {
				t.Fatalf("property = %q, want %q", property, test.wantProperty)
			}
		})
	}
}

func TestPropertyEscapesReservedWords(t *testing.T) {
	value, err := Property("class")
	if err != nil {
		t.Fatal(err)
	}
	if value != "classValue" {
		t.Fatalf("property = %q", value)
	}
}
