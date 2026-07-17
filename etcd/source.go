// Package etcd provides a typecfg Source backed by etcd KV.
// It lives in a separate module so the core typecfg module stays free of
// the etcd client dependency.
package etcd

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sinashahoveisi/typecfg"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const defaultRetryAfter = time.Second

// etcdAPI is the subset of clientv3 used by EtcdSource.
// Production uses *clientv3.Client; tests inject a fake.
type etcdAPI interface {
	Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error)
	Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan
}

// EtcdSource reads nested config from etcd under Prefix and watches for
// changes via clientv3's Watch API (revision-based, not polling).
type EtcdSource struct {
	// Client must be set for production use. Unlike Consul, etcd has no
	// safe zero-config default (endpoints are required).
	Client *clientv3.Client
	// Prefix is the key prefix to Get/Watch with WithPrefix.
	Prefix string

	// RetryAfter is the backoff after watch/get failures; zero means 1s.
	RetryAfter time.Duration

	api etcdAPI // nil -> Client; set by tests

	mu      sync.Mutex
	lastRev int64 // Header.Revision from most recent successful Load/Get
	hasLoad bool
}

// NewEtcdSource returns an EtcdSource for the given key prefix.
// Caller must still set Client before Load/Watch.
func NewEtcdSource(prefix string) *EtcdSource {
	return &EtcdSource{Prefix: prefix}
}

// Name returns "etcd:<Prefix>" for SourceError messages.
func (s *EtcdSource) Name() string { return "etcd:" + s.Prefix }

func (s *EtcdSource) getAPI() (etcdAPI, error) {
	if s.api != nil {
		return s.api, nil
	}
	if s.Client == nil {
		return nil, fmt.Errorf("etcd Client is required; construct with clientv3.New(clientv3.Config{Endpoints: ...})")
	}
	return s.Client, nil
}

// Load gets all keys under Prefix and builds a nested map; relative key
// path segments become nested maps. Records the response revision for Watch.
func (s *EtcdSource) Load(ctx context.Context) (map[string]any, error) {
	api, err := s.getAPI()
	if err != nil {
		return nil, &typecfg.SourceError{Source: s.Name(), Op: "fetch", Err: err}
	}
	raw, rev, err := s.getPrefix(ctx, api)
	if err != nil {
		return nil, &typecfg.SourceError{Source: s.Name(), Op: "fetch", Err: err}
	}
	s.mu.Lock()
	s.lastRev = rev
	s.hasLoad = true
	s.mu.Unlock()
	return raw, nil
}

func (s *EtcdSource) getPrefix(ctx context.Context, api etcdAPI) (map[string]any, int64, error) {
	resp, err := api.Get(ctx, s.Prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, 0, err
	}
	rev := int64(0)
	if resp != nil && resp.Header != nil {
		rev = resp.Header.Revision
	}
	return kvsToMap(s.Prefix, resp), rev, nil
}

func kvsToMap(prefix string, resp *clientv3.GetResponse) map[string]any {
	result := map[string]any{}
	if resp == nil {
		return result
	}
	for _, kv := range resp.Kvs {
		if kv == nil {
			continue
		}
		key := string(kv.Key)
		rest := key
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
		for i := range parts {
			parts[i] = strings.ToLower(parts[i])
		}
		insertNested(result, parts, string(kv.Value))
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

// Watch uses etcd's Watch API. If Load() already ran, watching starts at
// lastRev+1 so any revision advance between Load and Watch is delivered
// (no silent baseline reset). Without a prior Load, a Get establishes the
// current revision first (no signal), then Watch resumes from rev+1.
func (s *EtcdSource) Watch(ctx context.Context) (<-chan struct{}, func() error, error) {
	watchCtx, cancel := context.WithCancel(ctx)
	changed := make(chan struct{}, 1)

	retry := s.RetryAfter
	if retry <= 0 {
		retry = defaultRetryAfter
	}

	go func() {
		defer close(changed)
		defer cancel()

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

			api, err := s.getAPI()
			if err != nil {
				if !sleepOrDone(watchCtx, retry) {
					return
				}
				continue
			}

			s.mu.Lock()
			rev := s.lastRev
			sawLoad := s.hasLoad
			s.mu.Unlock()

			if !sawLoad {
				// Watch-only: establish current revision without signaling.
				_, newRev, err := s.getPrefix(watchCtx, api)
				if err != nil {
					if watchCtx.Err() != nil {
						return
					}
					if !sleepOrDone(watchCtx, retry) {
						return
					}
					continue
				}
				rev = newRev
				s.mu.Lock()
				s.lastRev = rev
				s.mu.Unlock()
			}

			fromRev := rev + 1
			opts := []clientv3.OpOption{clientv3.WithPrefix()}
			if fromRev > 0 {
				opts = append(opts, clientv3.WithRev(fromRev))
			}

			wch := api.Watch(watchCtx, s.Prefix, opts...)
			restart := false
			for wr := range wch {
				if watchCtx.Err() != nil {
					return
				}
				if err := wr.Err(); err != nil {
					// Compacted / connection drop: resync via Get, resume.
					_, newRev, gerr := s.getPrefix(watchCtx, api)
					if gerr == nil {
						s.mu.Lock()
						prev := s.lastRev
						s.lastRev = newRev
						s.hasLoad = true
						s.mu.Unlock()
						// If revision moved while the watch was down, notify
						// so the loader re-fetches (may have missed events).
						if newRev != prev {
							signal()
						}
					}
					restart = true
					break
				}
				if len(wr.Events) == 0 {
					continue
				}
				newRev := wr.Header.Revision
				s.mu.Lock()
				s.lastRev = newRev
				s.hasLoad = true
				s.mu.Unlock()
				signal()
			}

			if watchCtx.Err() != nil {
				return
			}
			if !restart {
				// Channel closed without a typed error — brief backoff then
				// re-Get and resume (same silent recovery convention).
				_, newRev, gerr := s.getPrefix(watchCtx, api)
				if gerr == nil {
					s.mu.Lock()
					prev := s.lastRev
					s.lastRev = newRev
					s.hasLoad = true
					s.mu.Unlock()
					if newRev != prev {
						signal()
					}
				}
				if !sleepOrDone(watchCtx, retry) {
					return
				}
			}
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

var _ typecfg.Source = (*EtcdSource)(nil)
var _ typecfg.Watchable = (*EtcdSource)(nil)
