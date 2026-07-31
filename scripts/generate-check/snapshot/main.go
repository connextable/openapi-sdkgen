package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"go.yaml.in/yaml/v4"
	compiler "openapi-sdkgen/internal/compiler"
	"openapi-sdkgen/internal/compiler/ir"
)

type repeatedStrings []string

func (values *repeatedStrings) String() string {
	return strings.Join(*values, ",")
}

func (values *repeatedStrings) Set(value string) error {
	if value == "" {
		return errors.New("option requires a non-empty value")
	}
	*values = append(*values, value)
	return nil
}

type snapshotReport struct {
	Input            string `json:"input"`
	Display          string `json:"display"`
	EffectiveBase    string `json:"effectiveBase"`
	SHA256           string `json:"sha256"`
	Bytes            int    `json:"bytes"`
	Title            string `json:"title"`
	Version          string `json:"version"`
	OpenAPIVersion   string `json:"openapiVersion"`
	OperationCount   int    `json:"operationCount"`
	ComponentCount   int    `json:"componentCount"`
	ComponentSchemas int    `json:"componentSchemaCount"`
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "openapi-sdkgen generate-check snapshot: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, warnings io.Writer) error {
	flags := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	input := flags.String("input", "", "OpenAPI input")
	dataOutput := flags.String("data-output", "", "snapshot byte output")
	baseOutput := flags.String("base-output", "", "effective base output")
	reportOutput := flags.String("report-output", "", "snapshot report output")
	var httpHeaderEnv repeatedStrings
	flags.Var(&httpHeaderEnv, "http-header-env", "HTTP header environment mapping")
	tlsClientCert := flags.String("tls-client-cert", "", "TLS client certificate")
	tlsClientKey := flags.String("tls-client-key", "", "TLS client key")
	tlsCAFile := flags.String("tls-ca-file", "", "additional TLS certificate authorities")
	offline := flags.Bool("offline", false, "disable network input")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse arguments: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if *input == "" || *dataOutput == "" || *baseOutput == "" || *reportOutput == "" {
		return errors.New("--input, --data-output, --base-output, and --report-output are required")
	}

	snapshot, err := compiler.AcquireInputSnapshot(*input, compiler.CompileOptions{
		InputReader:       stdin,
		Offline:           *offline,
		HTTPHeaderEnv:     httpHeaderEnv,
		TLSClientCert:     *tlsClientCert,
		TLSClientKey:      *tlsClientKey,
		TLSCAFile:         *tlsCAFile,
		HTTPWarningWriter: warnings,
	})
	if err != nil {
		return err
	}
	report, err := describeSnapshot(snapshot)
	if err != nil {
		return err
	}
	reportData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode snapshot report: %w", err)
	}
	reportData = append(reportData, '\n')
	if err := os.WriteFile(*dataOutput, snapshot.Data, 0o600); err != nil {
		return fmt.Errorf("write snapshot bytes: %w", err)
	}
	if err := os.WriteFile(*baseOutput, []byte(snapshot.EffectiveBase), 0o600); err != nil {
		return fmt.Errorf("write snapshot effective base: %w", err)
	}
	if err := os.WriteFile(*reportOutput, reportData, 0o600); err != nil {
		return fmt.Errorf("write snapshot report: %w", err)
	}
	return nil
}

func describeSnapshot(snapshot compiler.InputSnapshot) (snapshotReport, error) {
	var root map[string]any
	if err := yaml.Unmarshal(snapshot.Data, &root); err != nil {
		return snapshotReport{}, fmt.Errorf("parse snapshot metadata: %w", err)
	}
	info, _ := root["info"].(map[string]any)
	title, _ := info["title"].(string)
	version, _ := info["version"].(string)
	openapiVersion, _ := root["openapi"].(string)
	components, _ := root["components"].(map[string]any)
	componentCount := 0
	for _, value := range components {
		if entries, ok := value.(map[string]any); ok {
			componentCount += len(entries)
		}
	}
	schemas, _ := components["schemas"].(map[string]any)
	operationCount, err := countOperations(root)
	if err != nil {
		return snapshotReport{}, fmt.Errorf("count snapshot operations: %w", err)
	}
	digest := sha256.Sum256(snapshot.Data)
	return snapshotReport{
		Input:            snapshot.Input,
		Display:          snapshot.Display,
		EffectiveBase:    snapshot.EffectiveBase,
		SHA256:           hex.EncodeToString(digest[:]),
		Bytes:            len(snapshot.Data),
		Title:            title,
		Version:          version,
		OpenAPIVersion:   openapiVersion,
		OperationCount:   operationCount,
		ComponentCount:   componentCount,
		ComponentSchemas: len(schemas),
	}, nil
}

func countOperations(root map[string]any) (int, error) {
	total := 0
	for _, field := range []string{"paths", "webhooks"} {
		items, _ := root[field].(map[string]any)
		for name, value := range items {
			pathItem, _ := value.(map[string]any)
			count, err := countPathItemOperations(root, pathItem)
			if err != nil {
				return 0, fmt.Errorf("%s %q: %w", field, name, err)
			}
			total += count
		}
	}
	return total, nil
}

func countPathItemOperations(root, pathItem map[string]any) (int, error) {
	if reference, _ := pathItem["$ref"].(string); reference != "" {
		local, err := ir.IsLocalPathItemReference(reference)
		if err != nil {
			return 0, err
		}
		if !local {
			return countUnresolvedPathItemOperations(pathItem), nil
		}
	}
	resolved, err := ir.ResolvePathItem(root, pathItem)
	if err != nil {
		return 0, err
	}
	return countUnresolvedPathItemOperations(resolved), nil
}

func countUnresolvedPathItemOperations(pathItem map[string]any) int {
	total := 0
	for method := range pathItem {
		switch strings.ToLower(method) {
		case "get", "put", "post", "delete", "options", "head", "patch", "trace", "query":
			total++
		}
	}
	additional, _ := pathItem["additionalOperations"].(map[string]any)
	for _, value := range additional {
		if _, ok := value.(map[string]any); ok {
			total++
		}
	}
	return total
}
