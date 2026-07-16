// Package jsonschema provides an opt-in JSON Schema validator for
// typecfg.Loader via SetRawValidator. It lives in a separate module so
// the core typecfg module stays free of third-party dependencies.
package jsonschema

import (
	"bytes"
	"fmt"

	js "github.com/santhosh-tekuri/jsonschema/v5"
)

// Validator compiles schemaJSON once and returns a closure suitable for
// typecfg.Loader.SetRawValidator. The closure validates a merged raw
// config map against the compiled schema. Failures from Load surface as
// *typecfg.SchemaError wrapping the underlying library error.
func Validator(schemaJSON []byte) (func(map[string]any) error, error) {
	c := js.NewCompiler()
	const url = "schema.json"
	if err := c.AddResource(url, bytes.NewReader(schemaJSON)); err != nil {
		return nil, fmt.Errorf("jsonschema: add resource: %w", err)
	}
	sch, err := c.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("jsonschema: compile: %w", err)
	}
	return func(data map[string]any) error {
		return sch.Validate(data)
	}, nil
}
