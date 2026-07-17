package typecfg

import (
	"context"
	"os"
	"strings"
)

// EnvSource reads OS environment variables and turns them into a nested
// map, so APP_SERVER_PORT becomes {"server": {"port": "8080"}} (given
// prefix "APP" and delimiter "_"). Values stay as strings; type
// conversion happens during struct binding, where the target field's
// Go type is known.
type EnvSource struct {
	// Prefix filters which env vars are considered, e.g. "APP" only
	// picks up APP_*. Leave empty to consider all env vars (not
	// recommended outside of small tools).
	Prefix string
	// Delimiter splits an env var name into nested keys. Defaults to "_".
	Delimiter string
}

// NewEnvSource returns an EnvSource that only considers variables whose names
// start with prefix (plus the delimiter). An empty prefix considers all env
// vars — usually undesirable outside small tools.
func NewEnvSource(prefix string) *EnvSource {
	return &EnvSource{Prefix: prefix, Delimiter: "_"}
}

// Name returns "env:<prefix>*" for use in SourceError and FieldError sources.
func (e *EnvSource) Name() string { return "env:" + e.Prefix + "*" }

// Load scans the process environment into a nested map[string]any. Values
// remain strings; numeric/bool conversion happens during bind.
func (e *EnvSource) Load(_ context.Context) (map[string]any, error) {
	delim := e.Delimiter
	if delim == "" {
		delim = "_"
	}
	prefix := e.Prefix
	if prefix != "" && !strings.HasSuffix(prefix, delim) {
		prefix += delim
	}

	result := map[string]any{}
	for _, kv := range os.Environ() {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if prefix != "" {
			if !strings.HasPrefix(k, prefix) {
				continue
			}
			k = strings.TrimPrefix(k, prefix)
		}
		parts := strings.Split(strings.ToLower(k), strings.ToLower(delim))
		insertNested(result, parts, v)
	}
	return result, nil
}

func insertNested(m map[string]any, path []string, value string) {
	if len(path) == 0 {
		return
	}
	if len(path) == 1 {
		m[path[0]] = value
		return
	}
	next, ok := m[path[0]].(map[string]any)
	if !ok {
		next = map[string]any{}
		m[path[0]] = next
	}
	insertNested(next, path[1:], value)
}

var _ Source = (*EnvSource)(nil)
