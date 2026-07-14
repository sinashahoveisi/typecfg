package typecfg

import "context"

// Source is anything that can produce raw config key/value data: a YAML
// file, a JSON file, the environment, a remote key-value store, etc.
//
// Load returns a nested map (e.g. {"server": {"port": 8080}}) using
// lower_snake_case or dot-free keys matching struct tag names.
type Source interface {
	// Name identifies the source in error messages, e.g. "file:config.yaml".
	Name() string
	// Load reads and parses the source into a nested map[string]any.
	Load(ctx context.Context) (map[string]any, error)
}

// Watchable is an optional interface a Source can implement to support
// hot-reload. Watch should send on the returned channel whenever the
// underlying data may have changed (the loader will re-Load and diff).
// Closing stop must make the goroutine feeding the channel exit.
type Watchable interface {
	Watch(ctx context.Context) (changed <-chan struct{}, stop func() error, err error)
}

// mergeMaps merges src into dst (dst wins on nil, src overwrites dst on
// conflicting scalar keys; nested maps are merged recursively). Used to
// layer sources: e.g. yaml file as base, env vars as override.
func mergeMaps(dst, src map[string]any) map[string]any {
	if dst == nil {
		dst = map[string]any{}
	}
	for k, v := range src {
		if existing, ok := dst[k]; ok {
			existingMap, existingIsMap := existing.(map[string]any)
			newMap, newIsMap := v.(map[string]any)
			if existingIsMap && newIsMap {
				dst[k] = mergeMaps(existingMap, newMap)
				continue
			}
		}
		dst[k] = v
	}
	return dst
}
