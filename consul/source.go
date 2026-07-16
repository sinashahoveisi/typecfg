// Package consul provides a typecfg Source backed by HashiCorp Consul KV.
// It lives in a separate module so the core typecfg module stays free of
// the consul/api dependency.
package consul

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/sinashahoveisi/typecfg"
)

const (
	defaultWatchWait  = 5 * time.Minute
	defaultRetryAfter = time.Second
)

// kvLister is the subset of Consul's KV API used by ConsulSource.
// Production code uses *api.KV; tests inject a fake.
type kvLister interface {
	List(prefix string, q *api.QueryOptions) (api.KVPairs, *api.QueryMeta, error)
}

// ConsulSource reads nested config from Consul KV under Prefix and watches
// for changes via Consul blocking queries (WaitIndex).
type ConsulSource struct {
	Client *api.Client // nil -> api.NewClient(api.DefaultConfig())
	Prefix string      // e.g. "myapp/config/"

	// Optional overrides for tests / tuning. Zero means defaults.
	WaitTime   time.Duration // blocking query bound; default 5m
	RetryAfter time.Duration // backoff after connection errors; default 1s

	kv kvLister // nil -> Client.KV(); set by tests

	mu        sync.Mutex
	lastIndex uint64 // Consul LastIndex from most recent successful Load
	hasLoad   bool   // true after at least one successful Load
}

// NewConsulSource returns a ConsulSource for the given KV prefix.
func NewConsulSource(prefix string) *ConsulSource {
	return &ConsulSource{Prefix: prefix}
}

func (s *ConsulSource) Name() string { return "consul:" + s.Prefix }

func (s *ConsulSource) getKV() (kvLister, error) {
	if s.kv != nil {
		return s.kv, nil
	}
	client := s.Client
	if client == nil {
		c, err := api.NewClient(api.DefaultConfig())
		if err != nil {
			return nil, err
		}
		s.Client = c
		client = c
	}
	return client.KV(), nil
}

func (s *ConsulSource) Load(ctx context.Context) (map[string]any, error) {
	kv, err := s.getKV()
	if err != nil {
		return nil, &typecfg.SourceError{Source: s.Name(), Op: "fetch", Err: err}
	}
	opts := (&api.QueryOptions{}).WithContext(ctx)
	pairs, meta, err := kv.List(s.Prefix, opts)
	if err != nil {
		return nil, &typecfg.SourceError{Source: s.Name(), Op: "fetch", Err: err}
	}
	raw := pairsToMap(s.Prefix, pairs)

	// Remember Consul's raft index so Watch can detect KV changes that
	// occur between Load() and Watch() (same class of gap as RemoteHTTPSource).
	s.mu.Lock()
	s.hasLoad = true
	if meta != nil {
		s.lastIndex = meta.LastIndex
	} else {
		s.lastIndex = 0
	}
	s.mu.Unlock()

	return raw, nil
}

func pairsToMap(prefix string, pairs api.KVPairs) map[string]any {
	result := map[string]any{}
	for _, p := range pairs {
		if p == nil {
			continue
		}
		// Consul "folder" placeholders: key ends with "/" and empty value.
		if strings.HasSuffix(p.Key, "/") && len(p.Value) == 0 {
			continue
		}
		rest := p.Key
		if prefix != "" {
			if !strings.HasPrefix(rest, prefix) {
				continue
			}
			rest = strings.TrimPrefix(rest, prefix)
		}
		rest = strings.Trim(rest, "/")
		if rest == "" {
			continue
		}
		parts := strings.Split(rest, "/")
		// Match EnvSource: lowercase path segments for cfg tag alignment.
		for i := range parts {
			parts[i] = strings.ToLower(parts[i])
		}
		insertNested(result, parts, string(p.Value))
	}
	return result
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

// Watch uses Consul blocking queries. If Load() already ran, the first
// List compares meta.LastIndex against the index Load observed — an
// advance means signal immediately (do not silently absorb a Load→Watch
// gap). Without a prior Load, the first List only establishes the index.
func (s *ConsulSource) Watch(ctx context.Context) (<-chan struct{}, func() error, error) {
	watchCtx, cancel := context.WithCancel(ctx)
	changed := make(chan struct{}, 1)

	wait := s.WaitTime
	if wait <= 0 {
		wait = defaultWatchWait
	}
	retry := s.RetryAfter
	if retry <= 0 {
		retry = defaultRetryAfter
	}

	go func() {
		defer close(changed)
		defer cancel()

		kv, err := s.getKV()
		if err != nil {
			kv = nil
		}

		s.mu.Lock()
		loadIndex := s.lastIndex
		sawLoad := s.hasLoad
		s.mu.Unlock()

		var index uint64
		first := true

		signal := func() {
			select {
			case changed <- struct{}{}:
			default:
			}
		}

		for {
			if watchCtx.Err() != nil {
				return
			}
			if kv == nil {
				kv, err = s.getKV()
				if err != nil {
					if !sleepOrDone(watchCtx, retry) {
						return
					}
					continue
				}
			}

			opts := (&api.QueryOptions{
				WaitIndex: index,
				WaitTime:  wait,
			}).WithContext(watchCtx)

			_, meta, err := kv.List(s.Prefix, opts)
			if err != nil {
				if watchCtx.Err() != nil {
					return
				}
				kv = nil
				if !sleepOrDone(watchCtx, retry) {
					return
				}
				continue
			}
			if meta == nil {
				continue
			}

			newIndex := meta.LastIndex
			if newIndex == index {
				// Blocking query timed out with no change — loop again.
				continue
			}

			if first {
				first = false
				index = newIndex
				s.mu.Lock()
				s.lastIndex = newIndex
				s.mu.Unlock()
				// Prior Load saw loadIndex; current raft index already
				// moved → content changed between Load and Watch.
				if sawLoad && newIndex != loadIndex {
					signal()
				}
				// !sawLoad: Watch-only start; establish baseline, no signal.
				// sawLoad && same index: unchanged since Load; no signal.
				continue
			}

			index = newIndex
			s.mu.Lock()
			s.lastIndex = newIndex
			s.mu.Unlock()
			signal()
		}
	}()

	stop := func() error {
		cancel()
		return nil
	}
	return changed, stop, nil
}

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

var _ typecfg.Source = (*ConsulSource)(nil)
var _ typecfg.Watchable = (*ConsulSource)(nil)
