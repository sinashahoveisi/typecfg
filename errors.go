package typecfg

import "fmt"

// FieldError describes exactly what went wrong with a single config field,
// including where the value should have come from. This is the core
// difference from most config libraries: instead of "field X is required",
// you get "field X (env: APP_PORT, yaml: port) is required but was not set
// in config.yaml or in the environment".
//
// FieldError does not store the raw field value. For secret:"true" fields,
// callers must put only redacted text in Reason; Error() and
// ValidationError.Error() only format Field, Tag, Sources (key names), and
// Reason — they never look up source values.
type FieldError struct {
	// Field is the Go struct field path, e.g. "Server.Port".
	Field string
	// Tag is the struct tag key that failed (e.g. "required", "min", "oneof").
	Tag string
	// Sources lists where this field could have been populated from,
	// e.g. []string{"env:APP_PORT", "yaml:server.port"}. Key names only;
	// never values.
	Sources []string
	// Reason is a human-readable explanation. Must not contain secret field
	// values when the field is tagged secret:"true".
	Reason string
}

// Error implements the error interface for a single field failure.
func (e *FieldError) Error() string {
	if len(e.Sources) == 0 {
		return fmt.Sprintf("typecfg: field %q: %s", e.Field, e.Reason)
	}
	return fmt.Sprintf("typecfg: field %q (sources: %v): %s", e.Field, e.Sources, e.Reason)
}

// ValidationError aggregates all FieldErrors from one Load/reload so callers
// see every problem at once instead of fixing config one field at a time.
type ValidationError struct {
	// Errors is the full list of field-level failures from bind and/or validate.
	Errors []*FieldError
}

// Error formats a single FieldError or a multi-line list of them.
func (e *ValidationError) Error() string {
	if len(e.Errors) == 1 {
		return e.Errors[0].Error()
	}
	msg := fmt.Sprintf("typecfg: %d config errors found:\n", len(e.Errors))
	for _, fe := range e.Errors {
		msg += "  - " + fe.Error() + "\n"
	}
	return msg
}

// Unwrap returns each FieldError as an error for errors.Is / errors.As.
func (e *ValidationError) Unwrap() []error {
	errs := make([]error, len(e.Errors))
	for i, fe := range e.Errors {
		errs[i] = fe
	}
	return errs
}

// SourceError wraps a failure to read or parse a specific Source, naming the
// source and operation while preserving the underlying error via Unwrap.
type SourceError struct {
	// Source identifies the origin, e.g. "file:config.yaml" or "env:APP*".
	Source string
	// Op is the failing step, e.g. "read", "parse", "watch", or "fetch".
	Op string
	// Err is the underlying failure.
	Err error
}

// Error returns a "typecfg: <op> <source>: <err>" message.
func (e *SourceError) Error() string {
	return fmt.Sprintf("typecfg: %s %s: %v", e.Op, e.Source, e.Err)
}

// Unwrap returns the underlying Err.
func (e *SourceError) Unwrap() error { return e.Err }

// SchemaError wraps a failure from Loader.SetRawValidator (e.g. JSON Schema).
// It is distinct from SourceError (source I/O) and ValidationError (tags).
type SchemaError struct {
	// Err is the validator's underlying error.
	Err error
}

// Error returns a "typecfg: schema validation failed: …" message.
func (e *SchemaError) Error() string {
	return fmt.Sprintf("typecfg: schema validation failed: %v", e.Err)
}

// Unwrap returns the underlying Err.
func (e *SchemaError) Unwrap() error { return e.Err }
