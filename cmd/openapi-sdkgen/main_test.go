package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/connextable/openapi-sdkgen/internal/generator"
)

func TestGenerateReadsStandardInput(t *testing.T) {
	directory := t.TempDir()
	output := filepath.Join(directory, "generated-client")
	previous := standardInput
	standardInput = strings.NewReader(minimalDocument)
	t.Cleanup(func() { standardInput = previous })
	if err := run([]string{"generate", "--input", "-", "--target", "typescript", "--output", output}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(output, "index.ts")); err != nil {
		t.Fatalf("generated stdin SDK: %v", err)
	}
}

func TestGenerateDoesNotPublishOutputWhenInputFails(t *testing.T) {
	directory := t.TempDir()
	output := filepath.Join(directory, "generated-client")
	err := run([]string{"generate", "--input", "git://example.test/openapi.yaml", "--target", "typescript", "--output", output})
	if err == nil || !strings.Contains(err.Error(), "unsupported OpenAPI input scheme") {
		t.Fatalf("generate error = %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("failed input published output: %v", err)
	}
}

func TestGenerateDoesNotPublishOutputWhenHTTPInputFails(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	directory := t.TempDir()
	output := filepath.Join(directory, "generated-client")
	err := run([]string{"generate", "--input", server.URL + "/openapi.json", "--target", "typescript", "--output", output})
	if err == nil || !strings.Contains(err.Error(), "unexpected HTTP status") {
		t.Fatalf("generate error = %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("failed HTTP input published output: %v", err)
	}
}

func TestGenerateDoesNotPersistHTTPHeaderCredentialSentinel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows fails protected same-origin reference caching before persistence")
	}
	const sentinel = "credential-sentinel"
	t.Setenv("SDKGEN_CREDENTIAL_SENTINEL", sentinel)
	var successfulRequests int
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != sentinel {
			t.Errorf("Authorization = %q", got)
		}
		successfulRequests++
		switch request.URL.Path {
		case "/openapi.yaml":
			_, _ = response.Write([]byte(`openapi: 3.2.0
info: {title: Sentinel, version: "1"}
paths:
  /things:
    get:
      operationId: listThings
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema: {$ref: schemas.yaml#/Thing}
`))
		case "/schemas.yaml":
			_, _ = response.Write([]byte("Thing:\n  type: object\n  properties:\n    id: {type: string}\n"))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	directory := t.TempDir()
	output := filepath.Join(directory, "generated")
	lock := filepath.Join(directory, "refs.lock")
	previousError := standardError
	var diagnostics bytes.Buffer
	standardError = &diagnostics
	t.Cleanup(func() { standardError = previousError })
	args := []string{
		"generate", "--input", server.URL + "/openapi.yaml",
		"--http-header-env", "Authorization=SDKGEN_CREDENTIAL_SENTINEL",
		"--ref-lock", lock, "--update-ref-lock",
		"--target", "typescript", "--output", output,
	}
	if err := run(args); err != nil {
		t.Fatal(err)
	}
	if successfulRequests != 2 {
		t.Fatalf("authenticated requests = %d", successfulRequests)
	}
	assertDirectoryDoesNotContain(t, directory, sentinel)
	if strings.Contains(diagnostics.String(), sentinel) {
		t.Fatalf("diagnostics leaked sentinel: %q", diagnostics.String())
	}

	failedOutput := filepath.Join(directory, "failed")
	err := run([]string{
		"generate", "--input", server.URL + "/missing.yaml",
		"--http-header-env", "Authorization=SDKGEN_CREDENTIAL_SENTINEL",
		"--target", "typescript", "--output", failedOutput,
	})
	if err == nil {
		t.Fatal("missing authenticated input succeeded")
	}
	if strings.Contains(err.Error(), sentinel) || strings.Contains(diagnostics.String(), sentinel) {
		t.Fatalf("failure leaked sentinel: error=%q diagnostics=%q", err, diagnostics.String())
	}
	if _, statErr := os.Stat(failedOutput); !os.IsNotExist(statErr) {
		t.Fatalf("failed authenticated input published output: %v", statErr)
	}
}

func assertDirectoryDoesNotContain(t *testing.T, directory, forbidden string) {
	t.Helper()
	err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(contents), forbidden) {
			return fmt.Errorf("%s contains credential sentinel", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGenerateDoesNotPublishOutputWhenInputIsMalformed(t *testing.T) {
	directory := t.TempDir()
	output := filepath.Join(directory, "generated-client")
	previous := standardInput
	standardInput = strings.NewReader("not: [valid")
	t.Cleanup(func() { standardInput = previous })
	err := run([]string{"generate", "--input", "-", "--target", "typescript", "--output", output})
	if err == nil || !strings.Contains(err.Error(), "compile") {
		t.Fatalf("generate error = %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("malformed input published output: %v", err)
	}
}

func TestGenerateDoesNotPublishOutputWhenStandardInputIsEmptyOrOversized(t *testing.T) {
	previous := standardInput
	t.Cleanup(func() { standardInput = previous })
	for _, test := range []struct {
		name   string
		reader io.Reader
		want   string
	}{
		{name: "empty", reader: strings.NewReader(""), want: "empty"},
		{name: "oversized", reader: &repeatingInput{remaining: 64<<20 + 1}, want: "exceeds"},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			output := filepath.Join(directory, "generated-client")
			standardInput = test.reader
			err := run([]string{"generate", "--input", "-", "--target", "typescript", "--output", output})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("generate error = %v", err)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("failed stdin input published output: %v", err)
			}
		})
	}
}

type repeatingInput struct {
	remaining int
}

func (reader *repeatingInput) Read(buffer []byte) (int, error) {
	if reader.remaining == 0 {
		return 0, io.EOF
	}
	count := len(buffer)
	if count > reader.remaining {
		count = reader.remaining
	}
	for index := range buffer[:count] {
		buffer[index] = 'x'
	}
	reader.remaining -= count
	return count, nil
}

func TestGenerateWritesTypeScriptSourceTree(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "contract.json")
	if err := os.WriteFile(input, []byte(minimalDocument), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(directory, "generated-client")
	if err := run([]string{"generate", "--input", input, "--target", "typescript", "--output", output}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"index.ts", "metadata.ts", "generated/types.ts", "generated/client.ts", "generated/errors.ts", "generated/index.ts", "generated/runtime.ts"} {
		if _, err := os.Stat(filepath.Join(output, expected)); err != nil {
			t.Fatalf("missing %s: %v", expected, err)
		}
	}
	for _, forbidden := range []string{"package.json", "tsconfig.json", "manifest.json", "README.md"} {
		if _, err := os.Stat(filepath.Join(output, forbidden)); !os.IsNotExist(err) {
			t.Fatalf("source output unexpectedly contains %s: %v", forbidden, err)
		}
	}
}

func TestGenerateWithServerWritesOnlyExplicitServerEntry(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "contract.json")
	if err := os.WriteFile(input, []byte(minimalDocument), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(directory, "generated-client")
	if err := run([]string{"generate", "--input", input, "--target", "typescript", "--with", "server", "--output", output}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"server/runtime.ts", "server/webhooks.ts", "server/callbacks.ts"} {
		if _, err := os.Stat(filepath.Join(output, expected)); err != nil {
			t.Fatalf("missing %s: %v", expected, err)
		}
	}
	if _, err := os.Stat(filepath.Join(output, "server", "index.ts")); !os.IsNotExist(err) {
		t.Fatalf("server barrel unexpectedly exists: %v", err)
	}
	root, err := os.ReadFile(filepath.Join(output, "index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(root), "server") {
		t.Fatalf("client root imports server: %s", root)
	}
}

func TestGenerateRejectsRemovedPackageNameFlag(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "contract.json")
	if err := os.WriteFile(input, []byte(minimalDocument), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(directory, "generated")
	err := run([]string{"generate", "--input", input, "--target", "typescript", "--output", output, "--package-name", "@example/client"})
	if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("unexpected output stat error = %v", err)
	}
}

func TestGenerateRejectsUnknownTarget(t *testing.T) {
	err := run([]string{"generate", "--input", "contract.json", "--target", "kotlin", "--output", "out"})
	if err == nil || !strings.Contains(err.Error(), "unsupported SDK target") {
		t.Fatalf("error = %v", err)
	}
}

func TestGenerateRejectsRemovedJavaScriptTarget(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "openapi.json")
	output := filepath.Join(directory, "generated")
	if err := os.WriteFile(input, []byte(`{"openapi":"3.0.3","info":{"title":"Removed target","version":"1"},"paths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	err := run([]string{"generate", "--input", input, "--target", "javascript", "--output", output})
	if err == nil || !strings.Contains(err.Error(), "unsupported SDK target") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("unexpected output stat error = %v", err)
	}
}

func TestGenerateParsesRepeatableWithAddons(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "openapi.json")
	if err := os.WriteFile(input, []byte(`{"openapi":"3.0.3","info":{"title":"Add-on","version":"1"},"paths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown", args: []string{"--with", "worker"}, want: "unsupported SDK add-on"},
		{name: "duplicate", args: []string{"--with", "server", "--with", "server"}, want: "specified more than once"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := filepath.Join(directory, test.name)
			args := append([]string{"generate", "--input", input, "--target", "typescript", "--output", output}, test.args...)
			err := run(args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v", err)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("unexpected output stat error = %v", err)
			}
		})
	}
}

func TestGenerateAcceptsExplicitRemoteReferenceOptionsWithoutFetching(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "openapi.json")
	if err := os.WriteFile(input, []byte(`{"openapi":"3.1.0","info":{"title":"Offline","version":"1"},"paths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(directory, "generated")
	if err := run([]string{
		"generate", "--input", input, "--target", "typescript", "--output", output,
		"--allow-remote-ref", "https://schemas.example.test",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(output, "index.ts")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(input + ".openapi-sdkgen.lock"); !os.IsNotExist(err) {
		t.Fatalf("implicit lockfile stat error = %v", err)
	}
}

func TestRunRejectsInvalidArgumentsWithoutCreatingOutput(t *testing.T) {
	directory := t.TempDir()
	output := filepath.Join(directory, "output")
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown command", args: []string{"publish"}, want: "unknown command"},
		{name: "missing flags", args: []string{"generate"}, want: "required"},
		{name: "unexpected positional", args: []string{"generate", "extra"}, want: "unexpected arguments"},
		{name: "missing input", args: []string{"generate", "--input", filepath.Join(directory, "missing.json"), "--target", "typescript", "--output", output}, want: "compile"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := run(test.args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v", err)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("unexpected output stat error = %v", err)
			}
		})
	}
}

func TestSafeArtifactPathRejectsTraversal(t *testing.T) {
	for _, value := range []string{"", ".", "..", "../outside.ts", "/outside.ts"} {
		t.Run(value, func(t *testing.T) {
			if _, err := safeArtifactPath(value); err == nil {
				t.Fatalf("invalid path %q was accepted", value)
			}
		})
	}
}

func TestWriteArtifactsRejectsSymlinkOutput(t *testing.T) {
	directory := t.TempDir()
	outside := t.TempDir()
	output := filepath.Join(directory, "output")
	if err := os.Symlink(outside, output); err != nil {
		t.Fatal(err)
	}
	err := writeArtifacts(output, []generator.Artifact{{Path: "generated/client.ts", Data: []byte("export {}\n")}})
	if err == nil || !strings.Contains(err.Error(), "must not be a symlink") {
		t.Fatalf("writeArtifacts error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "generated", "client.ts")); !os.IsNotExist(err) {
		t.Fatalf("outside artifact stat error = %v", err)
	}
}

func TestWriteArtifactsRollsBackArtifactPathConflict(t *testing.T) {
	directory := t.TempDir()
	output := filepath.Join(directory, "output")
	err := writeArtifacts(output, []generator.Artifact{
		{Path: "generated", Data: []byte("not a directory\n")},
		{Path: "generated/client.ts", Data: []byte("export {}\n")},
	})
	if err == nil || !strings.Contains(err.Error(), "create artifact directory") {
		t.Fatalf("writeArtifacts error = %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("partial output stat error = %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(directory, ".openapi-sdkgen-output-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("staging directories = %v, %v", matches, err)
	}
}

func TestWriteArtifactsPreservesExistingOutputAndRejectsDuplicatePaths(t *testing.T) {
	directory := t.TempDir()
	existing := filepath.Join(directory, "existing")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(existing, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := writeArtifacts(existing, []generator.Artifact{{Path: "client.ts", Data: []byte("export {}\n")}})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing output error = %v", err)
	}
	if value, err := os.ReadFile(sentinel); err != nil || string(value) != "keep" {
		t.Fatalf("sentinel = %q, %v", value, err)
	}

	output := filepath.Join(directory, "duplicate")
	err = writeArtifacts(output, []generator.Artifact{
		{Path: "client.ts", Data: []byte("first\n")},
		{Path: "./client.ts", Data: []byte("second\n")},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate generated artifact") {
		t.Fatalf("duplicate artifact error = %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("duplicate output stat error = %v", err)
	}
}

const minimalDocument = `{
  "openapi": "3.2.0",
  "info": { "title": "Example API", "version": "1.2.3" },
  "paths": {
    "/health": {
      "get": {
        "operationId": "getHealth",
        "responses": {
          "200": {
            "description": "Healthy",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "required": ["status"],
                  "properties": { "status": { "type": "string" } }
                }
              }
            }
          }
        }
      }
    }
  }
}`
