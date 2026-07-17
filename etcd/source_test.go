package etcd

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sinashahoveisi/typecfg"
	"go.etcd.io/etcd/api/v3/etcdserverpb"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// Testing approach: fake etcdAPI (no etcd binary; go.etcd.io/etcd/tests
// integration harness is not present in this environment and would pull
// a full in-process etcd server + Go 1.24+ toolchain constraints from
// newer client modules). These tests validate nesting, SourceError,
// revision baseline / Load→Watch gap, and watch-loop recovery — they do
// NOT prove wire compatibility with a real etcd cluster.

type fakeEtcd struct {
	mu sync.Mutex

	kvs map[string]string
	rev int64
	err error

	// watchCtrl receives commands for active watchers.
	watchers []*fakeWatcher
}

type fakeWatcher struct {
	ch      chan clientv3.WatchResponse
	fromRev int64
	prefix  string
	ctx     context.Context
}

func newFakeEtcd() *fakeEtcd {
	return &fakeEtcd{kvs: map[string]string{}, rev: 1}
}

func (f *fakeEtcd) setKV(key, val string, rev int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.kvs[key] = val
	f.rev = rev
	f.err = nil
	// Notify watchers whose fromRev <= rev.
	for _, w := range f.watchers {
		if w.fromRev > 0 && rev >= w.fromRev {
			select {
			case w.ch <- clientv3.WatchResponse{
				Header: etcdserverpb.ResponseHeader{Revision: rev},
				Events: []*clientv3.Event{{
					Type: clientv3.EventTypePut,
					Kv:   &mvccpb.KeyValue{Key: []byte(key), Value: []byte(val)},
				}},
			}:
			default:
			}
		}
	}
}

func (f *fakeEtcd) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeEtcd) sendCompacted() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, w := range f.watchers {
		select {
		case w.ch <- clientv3.WatchResponse{CompactRevision: f.rev}:
		default:
		}
	}
}

// sendCanceled injects a generic watch failure (Canceled=true → wr.Err()
// non-nil, not ErrCompacted).
func (f *fakeEtcd) sendCanceled() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, w := range f.watchers {
		select {
		case w.ch <- clientv3.WatchResponse{Canceled: true}:
		default:
		}
	}
}

func (f *fakeEtcd) Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	_ = opts // WithPrefix assumed by EtcdSource
	resp := &clientv3.GetResponse{
		Header: &etcdserverpb.ResponseHeader{Revision: f.rev},
	}
	for k, v := range f.kvs {
		if strings.HasPrefix(k, key) {
			resp.Kvs = append(resp.Kvs, &mvccpb.KeyValue{
				Key:   []byte(k),
				Value: []byte(v),
			})
		}
	}
	return resp, nil
}

func (f *fakeEtcd) Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan {
	op := clientv3.OpGet(key, opts...)
	fromRev := op.Rev()
	ch := make(chan clientv3.WatchResponse, 8)
	w := &fakeWatcher{ch: ch, fromRev: fromRev, prefix: key, ctx: ctx}

	f.mu.Lock()
	f.watchers = append(f.watchers, w)
	// Replay gap: if store already advanced past fromRev, emit now.
	if fromRev > 0 && f.rev >= fromRev && f.err == nil {
		for k, v := range f.kvs {
			if strings.HasPrefix(k, key) {
				ch <- clientv3.WatchResponse{
					Header: etcdserverpb.ResponseHeader{Revision: f.rev},
					Events: []*clientv3.Event{{
						Type: clientv3.EventTypePut,
						Kv:   &mvccpb.KeyValue{Key: []byte(k), Value: []byte(v)},
					}},
				}
				break // one event is enough to signal change
			}
		}
	}
	f.mu.Unlock()

	go func() {
		<-ctx.Done()
		f.mu.Lock()
		// remove watcher
		out := f.watchers[:0]
		for _, x := range f.watchers {
			if x != w {
				out = append(out, x)
			}
		}
		f.watchers = out
		f.mu.Unlock()
		close(ch)
	}()

	return ch
}

func TestEtcdSource_Load_NestedMap(t *testing.T) {
	f := newFakeEtcd()
	f.setKV("myapp/config/server/port", "8080", 2)
	f.setKV("myapp/config/log/level", "debug", 2)

	src := &EtcdSource{Prefix: "myapp/config/", api: f}
	raw, err := src.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	server, ok := raw["server"].(map[string]any)
	if !ok {
		t.Fatalf("server = %#v", raw["server"])
	}
	if server["port"] != "8080" {
		t.Errorf("port = %#v", server["port"])
	}
	logm, _ := raw["log"].(map[string]any)
	if logm["level"] != "debug" {
		t.Errorf("level = %#v", logm["level"])
	}
	if !src.hasLoad || src.lastRev != 2 {
		t.Errorf("baseline rev = %d hasLoad=%v, want 2/true", src.lastRev, src.hasLoad)
	}
}

func TestEtcdSource_Load_DeepNesting(t *testing.T) {
	f := newFakeEtcd()
	f.setKV("app/cfg/a/b/c", "deep", 3)

	src := &EtcdSource{Prefix: "app/cfg/", api: f}
	raw, err := src.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	a := raw["a"].(map[string]any)
	b := a["b"].(map[string]any)
	if b["c"] != "deep" {
		t.Errorf("a.b.c = %#v", b["c"])
	}
}

func TestEtcdSource_Load_NilClientError(t *testing.T) {
	src := &EtcdSource{Prefix: "x/"}
	_, err := src.Load(context.Background())
	if err == nil {
		t.Fatal("expected error for nil Client")
	}
	var serr *typecfg.SourceError
	if !errors.As(err, &serr) {
		t.Fatalf("got %T: %v", err, err)
	}
	if serr.Op != "fetch" {
		t.Errorf("Op = %q", serr.Op)
	}
	if !strings.Contains(serr.Error(), "Client is required") {
		t.Errorf("Error() = %q", serr.Error())
	}
}

func TestEtcdSource_Load_SourceError(t *testing.T) {
	f := newFakeEtcd()
	f.setErr(errors.New("connection refused"))
	src := &EtcdSource{Prefix: "app/", api: f}
	_, err := src.Load(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var serr *typecfg.SourceError
	if !errors.As(err, &serr) || serr.Op != "fetch" {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(serr.Error(), "connection refused") {
		t.Errorf("Error() = %q", serr.Error())
	}
}

func TestEtcdSource_Watch_EventSignals(t *testing.T) {
	f := newFakeEtcd()
	f.setKV("p/x", "1", 5)

	src := &EtcdSource{Prefix: "p/", api: f, RetryAfter: 10 * time.Millisecond}
	// Establish baseline via Load so Watch starts at rev+1.
	if _, err := src.Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changed, stop, err := src.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer func() { _ = stop() }()

	select {
	case <-changed:
		t.Fatal("false positive before any change")
	case <-time.After(50 * time.Millisecond):
	}

	f.setKV("p/x", "2", 6)

	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("expected changed after put")
	}
}

func TestEtcdSource_Watch_NoFalsePositive(t *testing.T) {
	f := newFakeEtcd()
	f.setKV("p/x", "1", 5)

	src := &EtcdSource{Prefix: "p/", api: f, RetryAfter: 10 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	changed, stop, err := src.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer func() { _ = stop() }()

	deadline := time.After(120 * time.Millisecond)
	for {
		select {
		case <-changed:
			t.Fatal("changed fired without events")
		case <-deadline:
			return
		}
	}
}

func TestEtcdSource_Watch_ErrorResyncThenSignals(t *testing.T) {
	cases := []struct {
		name  string
		inject func(*fakeEtcd)
	}{
		{
			name: "compacted",
			inject: func(f *fakeEtcd) {
				// WatchResponse.CompactRevision != 0 → wr.Err() == ErrCompacted
				f.sendCompacted()
			},
		},
		{
			name: "generic_canceled",
			inject: func(f *fakeEtcd) {
				// WatchResponse.Canceled → wr.Err() non-nil (not ErrCompacted)
				f.sendCanceled()
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeEtcd()
			f.setKV("p/x", "1", 3)

			src := &EtcdSource{Prefix: "p/", api: f, RetryAfter: 20 * time.Millisecond}
			if _, err := src.Load(context.Background()); err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			changed, stop, err := src.Watch(ctx)
			if err != nil {
				t.Fatalf("Watch: %v", err)
			}
			defer func() { _ = stop() }()

			time.Sleep(30 * time.Millisecond)
			tc.inject(f)
			time.Sleep(40 * time.Millisecond)
			f.setKV("p/x", "9", 10)

			select {
			case <-changed:
			case <-time.After(2 * time.Second):
				t.Fatalf("watch did not recover and signal after %s + change", tc.name)
			}
		})
	}
}

func TestEtcdSource_Watch_CancelStopsCleanly(t *testing.T) {
	f := newFakeEtcd()
	f.setKV("p/x", "1", 1)
	src := &EtcdSource{Prefix: "p/", api: f, RetryAfter: 10 * time.Millisecond}

	ctx, cancel := context.WithCancel(context.Background())
	changed, stop, err := src.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	time.Sleep(40 * time.Millisecond)
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

type etcdPollConfig struct {
	Server struct {
		Port int `cfg:"port" validate:"required"`
	} `cfg:"server"`
}

func TestEtcdSource_Watch_DetectsChangeBetweenLoadAndWatch(t *testing.T) {
	f := newFakeEtcd()
	f.setKV("app/server/port", "8080", 10)

	src := &EtcdSource{Prefix: "app/", api: f, RetryAfter: 10 * time.Millisecond}
	loader := typecfg.New[etcdPollConfig](src)

	cfgA, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfgA.Server.Port != 8080 {
		t.Fatalf("Port = %d, want 8080", cfgA.Server.Port)
	}

	// Change AFTER Load, BEFORE Watch (revision advances).
	f.setKV("app/server/port", "7777", 11)

	reloads := make(chan *etcdPollConfig, 2)
	loader.OnReload(func(old, new *etcdPollConfig) { reloads <- new })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := loader.Watch(ctx); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer func() { _ = loader.Stop() }()

	select {
	case cfgB := <-reloads:
		if cfgB.Server.Port != 7777 {
			t.Errorf("OnReload Port = %d, want 7777", cfgB.Server.Port)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnReload never fired for Load→Watch gap — baseline absorbed config B")
	}
}

func TestEtcdSource_Name(t *testing.T) {
	if NewEtcdSource("myapp/").Name() != "etcd:myapp/" {
		t.Fatal(NewEtcdSource("myapp/").Name())
	}
}
