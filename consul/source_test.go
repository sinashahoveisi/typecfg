package consul

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/sinashahoveisi/typecfg"
)

// Testing approach: fake kvLister (no Consul binary / embeddable agent in
// this environment). These tests validate ConsulSource's key nesting,
// SourceError wrapping, and watch-loop shape — they do NOT prove wire
// compatibility with a real Consul server.

type fakeKV struct {
	mu sync.Mutex

	pairs api.KVPairs
	index uint64
	err   error

	// When WaitIndex > 0, List blocks until release is closed or ctx done.
	release chan struct{}
	// listCalls counts List invocations (for assertions).
	listCalls int
}

func (f *fakeKV) set(pairs api.KVPairs, index uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pairs = pairs
	f.index = index
	f.err = nil
}

func (f *fakeKV) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeKV) List(prefix string, q *api.QueryOptions) (api.KVPairs, *api.QueryMeta, error) {
	f.mu.Lock()
	f.listCalls++
	err := f.err
	pairs := f.pairs
	index := f.index
	release := f.release
	f.mu.Unlock()

	if err != nil {
		return nil, nil, err
	}

	waitIndex := uint64(0)
	var ctx context.Context
	if q != nil {
		waitIndex = q.WaitIndex
		ctx = q.Context()
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if waitIndex > 0 {
		// Simulate blocking query: wait for release (index change) or ctx.
		if release != nil {
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-release:
			case <-time.After(30 * time.Millisecond):
				// Timeout with no change — return same index.
				f.mu.Lock()
				index = f.index
				pairs = f.pairs
				f.mu.Unlock()
				return pairs, &api.QueryMeta{LastIndex: index}, nil
			}
		} else {
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(20 * time.Millisecond):
				return pairs, &api.QueryMeta{LastIndex: index}, nil
			}
		}
		// After release, re-read current state.
		f.mu.Lock()
		pairs = f.pairs
		index = f.index
		err = f.err
		f.mu.Unlock()
		if err != nil {
			return nil, nil, err
		}
	}

	return pairs, &api.QueryMeta{LastIndex: index}, nil
}

func TestConsulSource_Load_NestedMap(t *testing.T) {
	kv := &fakeKV{}
	kv.set(api.KVPairs{
		{Key: "myapp/config/server/port", Value: []byte("8080")},
		{Key: "myapp/config/log/level", Value: []byte("debug")},
	}, 1)

	src := &ConsulSource{Prefix: "myapp/config/", kv: kv}
	raw, err := src.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	server, ok := raw["server"].(map[string]any)
	if !ok {
		t.Fatalf("server = %#v", raw["server"])
	}
	if server["port"] != "8080" {
		t.Errorf("port = %#v, want \"8080\"", server["port"])
	}
	logm, _ := raw["log"].(map[string]any)
	if logm["level"] != "debug" {
		t.Errorf("level = %#v, want debug", logm["level"])
	}
}

func TestConsulSource_Load_DeepNesting(t *testing.T) {
	kv := &fakeKV{}
	kv.set(api.KVPairs{
		{Key: "app/cfg/a/b/c", Value: []byte("deep")},
	}, 1)

	src := &ConsulSource{Prefix: "app/cfg/", kv: kv}
	raw, err := src.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	a, ok := raw["a"].(map[string]any)
	if !ok {
		t.Fatalf("a = %#v", raw["a"])
	}
	b, ok := a["b"].(map[string]any)
	if !ok {
		t.Fatalf("b = %#v", a["b"])
	}
	if b["c"] != "deep" {
		t.Errorf("a.b.c = %#v, want deep", b["c"])
	}
}

func TestConsulSource_Load_SourceError(t *testing.T) {
	kv := &fakeKV{}
	kv.setErr(errors.New("connection refused"))

	src := &ConsulSource{Prefix: "myapp/", kv: kv}
	_, err := src.Load(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var serr *typecfg.SourceError
	if !errors.As(err, &serr) {
		t.Fatalf("got %T: %v", err, err)
	}
	if serr.Op != "fetch" {
		t.Errorf("Op = %q, want fetch", serr.Op)
	}
	if !strings.Contains(serr.Error(), "connection refused") {
		t.Errorf("Error() = %q", serr.Error())
	}
	if !strings.HasPrefix(serr.Source, "consul:") {
		t.Errorf("Source = %q", serr.Source)
	}
}

func TestConsulSource_Watch_IndexChangeSignals(t *testing.T) {
	release := make(chan struct{})
	kv := &fakeKV{release: release, index: 10, pairs: api.KVPairs{
		{Key: "p/x", Value: []byte("1")},
	}}

	src := &ConsulSource{
		Prefix:     "p/",
		kv:         kv,
		WaitTime:   50 * time.Millisecond,
		RetryAfter: 10 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	changed, stop, err := src.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer func() { _ = stop() }()

	// First List establishes index 10 — must not signal yet.
	select {
	case <-changed:
		t.Fatal("false positive signal on initial index establish")
	case <-time.After(80 * time.Millisecond):
	}

	// Advance index and unblock the blocking query.
	kv.mu.Lock()
	kv.index = 11
	kv.pairs = api.KVPairs{{Key: "p/x", Value: []byte("2")}}
	kv.mu.Unlock()
	close(release)

	select {
	case <-changed:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("expected changed signal after index advance")
	}
}

type consulPollConfig struct {
	Server struct {
		Port int `cfg:"port" validate:"required"`
	} `cfg:"server"`
}

// TestConsulSource_Watch_DetectsChangeBetweenLoadAndWatch mirrors
// TestRemoteHTTP_Watch_DetectsChangeBetweenLoadAndWatch: KV that changes
// after Load and before Watch must still reach OnReload.
func TestConsulSource_Watch_DetectsChangeBetweenLoadAndWatch(t *testing.T) {
	kv := &fakeKV{}
	kv.set(api.KVPairs{
		{Key: "app/server/port", Value: []byte("8080")},
	}, 10)

	src := &ConsulSource{
		Prefix:     "app/",
		kv:         kv,
		WaitTime:   50 * time.Millisecond,
		RetryAfter: 10 * time.Millisecond,
	}
	loader := typecfg.New[consulPollConfig](src)

	cfgA, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfgA.Server.Port != 8080 {
		t.Fatalf("config A Port = %d, want 8080", cfgA.Server.Port)
	}

	// Change KV AFTER Load, BEFORE Watch (index must advance).
	kv.set(api.KVPairs{
		{Key: "app/server/port", Value: []byte("7777")},
	}, 11)

	reloads := make(chan *consulPollConfig, 2)
	loader.OnReload(func(old, new *consulPollConfig) { reloads <- new })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := loader.Watch(ctx); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer func() { _ = loader.Stop() }()

	select {
	case cfgB := <-reloads:
		if cfgB.Server.Port != 7777 {
			t.Errorf("OnReload Port = %d, want 7777 (change between Load and Watch)", cfgB.Server.Port)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnReload never fired for change between Load and Watch — baseline absorbed config B")
	}
}

func TestConsulSource_Watch_NoFalsePositiveOnTimeout(t *testing.T) {
	// No release channel: blocking queries time out and return same index.
	kv := &fakeKV{index: 5, pairs: api.KVPairs{
		{Key: "p/x", Value: []byte("1")},
	}}

	src := &ConsulSource{
		Prefix:   "p/",
		kv:       kv,
		WaitTime: 15 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	changed, stop, err := src.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer func() { _ = stop() }()

	deadline := time.After(150 * time.Millisecond)
	for {
		select {
		case <-changed:
			t.Fatal("changed fired without index change")
		case <-deadline:
			return
		}
	}
}

func TestConsulSource_Watch_SurvivesConnectionErrorThenSignals(t *testing.T) {
	release := make(chan struct{})
	kv := &fakeKV{
		release: release,
		index:   1,
		pairs:   api.KVPairs{{Key: "p/x", Value: []byte("1")}},
	}

	src := &ConsulSource{
		Prefix:     "p/",
		kv:         kv,
		WaitTime:   50 * time.Millisecond,
		RetryAfter: 20 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	changed, stop, err := src.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer func() { _ = stop() }()

	// Let initial index establish.
	time.Sleep(40 * time.Millisecond)

	// Inject a transient error for the next List(s).
	kv.setErr(errors.New("temporary network blip"))
	time.Sleep(60 * time.Millisecond)

	// Recover and advance index.
	kv.mu.Lock()
	kv.err = nil
	kv.index = 2
	kv.pairs = api.KVPairs{{Key: "p/x", Value: []byte("2")}}
	kv.mu.Unlock()
	close(release)

	select {
	case <-changed:
		// watch survived and picked up the change
	case <-time.After(2 * time.Second):
		t.Fatal("watch did not recover and signal after transient error")
	}
}

func TestConsulSource_Watch_CancelStopsCleanly(t *testing.T) {
	kv := &fakeKV{index: 1, pairs: api.KVPairs{
		{Key: "p/x", Value: []byte("1")},
	}}
	src := &ConsulSource{
		Prefix:   "p/",
		kv:       kv,
		WaitTime: 30 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())

	changed, stop, err := src.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	cancel()
	_ = stop()

	closed := make(chan struct{})
	go func() {
		for range changed {
			continue // drain until closed
		}
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("changed channel did not close after cancel")
	}
}

func TestConsulSource_Name(t *testing.T) {
	src := NewConsulSource("myapp/config/")
	if src.Name() != "consul:myapp/config/" {
		t.Errorf("Name() = %q", src.Name())
	}
}

func TestPairsToMap_SkipsFolderKeys(t *testing.T) {
	raw := pairsToMap("myapp/", api.KVPairs{
		{Key: "myapp/", Value: nil},
		{Key: "myapp/server/", Value: []byte{}},
		{Key: "myapp/server/port", Value: []byte("80")},
	})
	server, ok := raw["server"].(map[string]any)
	if !ok {
		t.Fatalf("raw = %#v", raw)
	}
	if server["port"] != "80" {
		t.Errorf("port = %#v", server["port"])
	}
}
