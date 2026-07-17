package typecfg

// GeneratedBinder is implemented by typecfg-gen output. It replaces the
// reflection bind/validate walk with direct field assignment while producing
// the same FieldError shapes and messages.
type GeneratedBinder[T any] interface {
	// BindGenerated fills a new T from raw (the merged source map), returning
	// setFields (cfg-tag paths present or defaulted) and any type-conversion
	// FieldErrors. It must not run validate rules.
	BindGenerated(raw map[string]any) (*T, map[string]struct{}, []*FieldError)
	// ValidateGenerated applies validate tags (and Validator, if implemented)
	// using setFields from BindGenerated. raw is the same merged map so
	// required-field "did you mean" suggestions match the reflection path.
	ValidateGenerated(cfg *T, setFields map[string]struct{}, raw map[string]any) []*FieldError
}

// NewGenerated returns a Loader that uses binder for bind/validate instead of
// reflection. Source merging order, SetRawValidator timing, Watch/OnReload/
// OnError/SetLogger/Get, and returned error types match New[T] exactly —
// only the bind/validate implementation differs.
func NewGenerated[T any](binder GeneratedBinder[T], sources ...Source) *Loader[T] {
	return &Loader[T]{sources: sources, binder: binder}
}
