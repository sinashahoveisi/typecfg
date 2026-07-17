package typecfg

// GeneratedBinder is implemented by code emitted by typecfg-gen. It binds and
// validates a config struct without reflecting over T's fields.
//
// ValidateGenerated receives the same merged raw map Load passed to
// BindGenerated so required-field "did you mean" suggestions match the
// reflection path (validate.go).
type GeneratedBinder[T any] interface {
	BindGenerated(raw map[string]any) (*T, map[string]struct{}, []*FieldError)
	ValidateGenerated(cfg *T, setFields map[string]struct{}, raw map[string]any) []*FieldError
}

// NewGenerated creates a Loader that uses binder for bind/validate instead of
// reflection. Source merging, Watch/OnReload/OnError/SetLogger/Get,
// SetRawValidator, and error types behave identically to New[T].
func NewGenerated[T any](binder GeneratedBinder[T], sources ...Source) *Loader[T] {
	return &Loader[T]{sources: sources, binder: binder}
}
