// Command openapi31validate checks an OpenAPI document against the OAS 3.1.0
// meta-schema (https://spec.openapis.org/oas/3.1/schema/2022-10-07).
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: openapi31validate <oas-3.1.schema.json> <spec.yaml>")
		os.Exit(2)
	}
	schema, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	spec, err := os.ReadFile(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := validate(schema, spec); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if errors.Is(err, errInvalid) {
			os.Exit(1)
		}
		os.Exit(2)
	}
	fmt.Println("OpenAPI 3.1 schema: ok")
}

var errInvalid = errors.New("openapi 3.1 schema")

func validate(schema, spec []byte) error {
	schemaDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	if err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	inst, err := yamlValue(spec)
	if err != nil {
		return err
	}
	c := jsonschema.NewCompiler()
	const id = "https://spec.openapis.org/oas/3.1/schema/2022-10-07"
	if err := c.AddResource(id, schemaDoc); err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	sch, err := c.Compile(id)
	if err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	err = sch.Validate(inst)
	if err == nil {
		return nil
	}
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return err
	}
	leaves := leafErrors(ve)
	var b strings.Builder
	fmt.Fprintf(&b, "OpenAPI 3.1 schema: %d error(s)\n", len(leaves))
	for i, leaf := range leaves {
		fmt.Fprintf(&b, "%d. %s\n", i+1, strings.TrimSpace(leaf))
	}
	return fmt.Errorf("%w\n%s", errInvalid, strings.TrimRight(b.String(), "\n"))
}

func yamlValue(spec []byte) (any, error) {
	var doc any
	if err := yaml.Unmarshal(spec, &doc); err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}
	return inst, nil
}

func leafErrors(e *jsonschema.ValidationError) []string {
	if len(e.Causes) == 0 {
		return []string{e.Error()}
	}
	var out []string
	for _, cause := range e.Causes {
		out = append(out, leafErrors(cause)...)
	}
	return out
}
