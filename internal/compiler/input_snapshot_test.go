package sdkgen

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcquireInputSnapshotReadsLocalRootOnceAndKeepsExactBytes(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "openapi.json")
	original := []byte(`{"openapi":"3.2.0","info":{"title":"Original","version":"1"},"paths":{}}`)
	changed := []byte(`{"openapi":"3.2.0","info":{"title":"Changed","version":"2"},"paths":{}}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	previousReader := readInputFile
	t.Cleanup(func() { readInputFile = previousReader })
	reads := 0
	readInputFile = func(name string) ([]byte, error) {
		reads++
		data, err := os.ReadFile(name)
		if err == nil {
			err = os.WriteFile(name, changed, 0o600)
		}
		return data, err
	}

	snapshot, err := AcquireInputSnapshot(path, CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if reads != 1 {
		t.Fatalf("root reads = %d, want 1", reads)
	}
	if string(snapshot.Data) != string(original) || snapshot.EffectiveBase != path || snapshot.Display != path {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	document, err := CompileInputWithOptions("-", CompileOptions{
		InputReader: strings.NewReader(string(snapshot.Data)),
		InputBase:   snapshot.EffectiveBase,
	})
	if err != nil {
		t.Fatal(err)
	}
	info, _ := document.Raw["info"].(map[string]any)
	if info["title"] != "Original" {
		t.Fatalf("compiled title = %v, want Original", info["title"])
	}
}

func TestAcquireInputSnapshotReportsFinalRedirectURLAndFetchesOnce(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		if request.URL.Path == "/root/openapi.json" {
			http.Redirect(response, request, "/final/openapi.json", http.StatusFound)
			return
		}
		_, _ = response.Write([]byte(`{"openapi":"3.2.0","info":{"title":"Redirected","version":"1"},"paths":{}}`))
	}))
	defer server.Close()

	snapshot, err := AcquireInputSnapshot(server.URL+"/root/openapi.json?token=secret", CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("HTTP requests = %d, want one root fetch plus one redirect response", requests)
	}
	if snapshot.EffectiveBase != server.URL+"/final/openapi.json" || snapshot.Display != snapshot.EffectiveBase {
		t.Fatalf("redirect snapshot = %#v", snapshot)
	}
	if strings.Contains(snapshot.Input, "secret") || snapshot.Input != server.URL+"/root/openapi.json" {
		t.Fatalf("snapshot input display leaked query data: %q", snapshot.Input)
	}
}

func TestCompileStdinHTTPBaseReusesTrustedTransportForRelativeReferences(t *testing.T) {
	const token = "snapshot-token"
	t.Setenv("SDKGEN_SNAPSHOT_TOKEN", token)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		if request.URL.Path != "/final/schema.json" {
			t.Errorf("reference path = %q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != token {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		_, _ = response.Write([]byte(`{"Thing":{"type":"string"}}`))
	}))
	defer server.Close()

	document := `{
	  "openapi":"3.2.0",
	  "info":{"title":"Stdin","version":"1"},
	  "paths":{},
	  "components":{"schemas":{"Thing":{"$ref":"./schema.json#/Thing"}}}
	}`
	_, err := CompileInputWithOptions("-", CompileOptions{
		InputReader:       strings.NewReader(document),
		InputBase:         server.URL + "/final/openapi.json",
		HTTPHeaderEnv:     []string{"Authorization=SDKGEN_SNAPSHOT_TOKEN"},
		HTTPWarningWriter: &strings.Builder{},
		RefLockPath:       filepath.Join(t.TempDir(), "references.lock"),
		UpdateRefLock:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("relative reference requests = %d, want 1", requests)
	}
}

func TestCompileStdinRejectsHTTPTransportSettingsWithoutHTTPBase(t *testing.T) {
	t.Setenv("SDKGEN_SNAPSHOT_TOKEN", "token")
	document := `{"openapi":"3.2.0","info":{"title":"Stdin","version":"1"},"paths":{}}`
	for _, base := range []string{"", filepath.Join(t.TempDir(), "openapi.json")} {
		_, err := CompileInputWithOptions("-", CompileOptions{
			InputReader:   strings.NewReader(document),
			InputBase:     base,
			HTTPHeaderEnv: []string{"Authorization=SDKGEN_SNAPSHOT_TOKEN"},
		})
		if err == nil || !strings.Contains(err.Error(), "only valid with an HTTP(S)") {
			t.Fatalf("base %q error = %v", base, err)
		}
	}
}
