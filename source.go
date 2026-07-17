package typecfg

import "context"

// Source produces raw nested config data (e.g. YAML file, env vars, Consul).
// Implementations should return maps whose keys match cfg struct tags;
// values may be nested map[string]any or scalars (often strings from env).
type Source interface {
	// Name returns a short label for errors, e.g. "file:config.yaml".
	Name() string
	// Load fetches and parses the source. Returning an error aborts Loader.Load
	// immediately (no bind/validate); Watch reloads treat that as OnError.
	Load(ctx context.Context) (map[string]any, error)
}

// Watchable is implemented by Sources that can signal when underlying data
// may have changed. Loader.Watch calls Watch on each such source and
// aggregates change notifications into a single reload loop.
type Watchable interface {
	// Watch starts a background signaler. changed receives a value when a
	// reload should be attempted (coalesced by Loader). stop must cancel
	// the watcher and wait until it exits; ctx cancellation should also stop it.
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
