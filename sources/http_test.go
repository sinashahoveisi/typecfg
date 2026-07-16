package sources

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sinashahoveisi/typecfg"
)

func TestRemoteHTTP_LoadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"server":{"port":8080},"log":{"level":"debug"}}`))
	}))
	defer srv.Close()

	src := NewRemoteHTTPSource(srv.URL)
	raw, err := src.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	server, ok := raw["server"].(map[string]any)
	if !ok {
		t.Fatalf("server = %#v", raw["server"])
	}
	port, ok := server["port"].(float64)
	if !ok || port != 8080 {
		t.Errorf("port = %#v, want 8080", server["port"])
	}
	logm, _ := raw["log"].(map[string]any)
	if logm["level"] != "debug" {
		t.Errorf("level = %#v, want debug", logm["level"])
	}
}

func TestRemoteHTTP_LoadYAML_ContentTypeInference(t *testing.T) {
	for _, ct := range []string{"text/yaml", "application/yaml"} {
		t.Run(ct, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", ct)
				_, _ = w.Write([]byte("server:\n  port: 9090\nlog:\n  level: info\n"))
			}))
			defer srv.Close()

			raw, err := NewRemoteHTTPSource(srv.URL).Load(context.Background())
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			server, ok := raw["server"].(map[string]any)
			if !ok {
				t.Fatalf("server = %#v", raw["server"])
			}
			port, ok := server["port"].(int)
			if !ok || port != 9090 {
				t.Errorf("port = %#v, want 9090", server["port"])
			}
		})
	}
}

func TestRemoteHTTP_Non2xx_SourceErrorTruncatedBody(t *testing.T) {
	longBody := strings.Repeat("X", 500)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(longBody))
	}))
	defer srv.Close()

	_, err := NewRemoteHTTPSource(srv.URL).Load(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var serr *typecfg.SourceError
	if !errors.As(err, &serr) {
		t.Fatalf("got %T: %v", err, err)
	}
	msg := serr.Error()
	if !strings.Contains(msg, "500") {
		t.Errorf("message should include status code, got %q", msg)
	}
	if !strings.Contains(serr.Err.Error(), "HTTP 500:") {
		t.Errorf("Err = %q", serr.Err)
	}
	// Truncated body: at most httpErrorBodyCap of 'X', not the full 500.
	xs := strings.Count(serr.Err.Error(), "X")
	if xs > httpErrorBodyCap {
		t.Errorf("body snippet has %d X runes, want <= %d", xs, httpErrorBodyCap)
	}
	if xs < httpErrorBodyCap {
		t.Errorf("expected full cap of %d X in snippet, got %d", httpErrorBodyCap, xs)
	}
	if strings.Contains(msg, longBody) {
		t.Error("error message must not include the full uncapped body")
	}
}

func TestRemoteHTTP_FormatOverridesContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ambiguous / missing Content-Type; body is JSON.
		_, _ = w.Write([]byte(`{"ok":true,"n":1}`))
	}))
	defer srv.Close()

	src := &RemoteHTTPSource{URL: srv.URL, Format: "json"}
	raw, err := src.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if raw["ok"] != true {
		t.Errorf("ok = %#v, want true", raw["ok"])
	}
}

func TestRemoteHTTP_CustomHeaders(t *testing.T) {
	var gotAuth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"a":1}`))
	}))
	defer srv.Close()

	src := &RemoteHTTPSource{
		URL:     srv.URL,
		Headers: map[string]string{"Authorization": "Bearer test-token"},
	}
	if _, err := src.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if gotAuth.Load() != "Bearer test-token" {
		t.Errorf("Authorization = %v, want Bearer test-token", gotAuth.Load())
	}
}

type httpPollConfig struct {
	Server struct {
		Port int `cfg:"port" validate:"required"`
	} `cfg:"server"`
}

func TestRemoteHTTP_Watch_SignalsOnRealChange(t *testing.T) {
	var mu sync.Mutex
	body := `{"server":{"port":8080}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	src := &RemoteHTTPSource{URL: srv.URL, PollInterval: 50 * time.Millisecond}
	loader := typecfg.New[httpPollConfig](src)
	if _, err := loader.Load(context.Background()); err != nil {
		t.Fatalf("initial Load: %v", err)
	}

	reloads := make(chan *httpPollConfig, 4)
	loader.OnReload(func(old, new *httpPollConfig) { reloads <- new })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := loader.Watch(ctx); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer loader.Stop()

	// Content still A during Watch start; change after first poll settles.
	time.Sleep(80 * time.Millisecond)
	mu.Lock()
	body = `{"server":{"port":9999}}`
	mu.Unlock()

	select {
	case cfg := <-reloads:
		if cfg.Server.Port != 9999 {
			t.Errorf("Port = %d, want 9999", cfg.Server.Port)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for OnReload after content change")
	}

	// Exactly one reload for this change (no more within a couple of ticks).
	select {
	case cfg := <-reloads:
		t.Fatalf("unexpected extra reload: %+v", cfg)
	case <-time.After(200 * time.Millisecond):
	}
}

// TestRemoteHTTP_Watch_DetectsChangeBetweenLoadAndWatch covers the realistic
// gap in examples/basic (Load, then later Watch): content that changes after
// Load returns but before Watch starts must still reach OnReload.
func TestRemoteHTTP_Watch_DetectsChangeBetweenLoadAndWatch(t *testing.T) {
	var mu sync.Mutex
	body := `{"server":{"port":8080}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	src := &RemoteHTTPSource{URL: srv.URL, PollInterval: 50 * time.Millisecond}
	loader := typecfg.New[httpPollConfig](src)

	cfgA, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfgA.Server.Port != 8080 {
		t.Fatalf("config A Port = %d, want 8080", cfgA.Server.Port)
	}

	// Change remote content AFTER Load, BEFORE Watch.
	mu.Lock()
	body = `{"server":{"port":7777}}`
	mu.Unlock()

	reloads := make(chan *httpPollConfig, 2)
	loader.OnReload(func(old, new *httpPollConfig) { reloads <- new })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := loader.Watch(ctx); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer loader.Stop()

	select {
	case cfgB := <-reloads:
		if cfgB.Server.Port != 7777 {
			t.Errorf("OnReload Port = %d, want 7777 (change between Load and Watch)", cfgB.Server.Port)
		}
		if loader.Get().Server.Port != 7777 {
			t.Errorf("Get() Port = %d, want 7777", loader.Get().Server.Port)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnReload never fired for change between Load and Watch — baseline absorbed config B")
	}
}

func TestRemoteHTTP_Watch_NoSignalWhenUnchanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"server":{"port":8080}}`))
	}))
	defer srv.Close()

	src := &RemoteHTTPSource{URL: srv.URL, PollInterval: 40 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	changed, stop, err := src.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer stop()

	// Wait through several ticks; baseline must not produce a signal.
	deadline := time.After(250 * time.Millisecond)
	for {
		select {
		case _, ok := <-changed:
			if !ok {
				t.Fatal("changed channel closed unexpectedly")
			}
			t.Fatal("changed fired despite unchanged content")
		case <-deadline:
			return
		}
	}
}

func TestRemoteHTTP_Watch_SurvivesTransientErrorThenPicksUpChange(t *testing.T) {
	var mu sync.Mutex
	failOnce := true
	body := `{"server":{"port":8080}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if failOnce {
			failOnce = false
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("blip"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	src := &RemoteHTTPSource{URL: srv.URL, PollInterval: 50 * time.Millisecond}
	loader := typecfg.New[httpPollConfig](src)

	// Initial load with healthy response (disable fail for this call).
	mu.Lock()
	failOnce = false
	mu.Unlock()
	if _, err := loader.Load(context.Background()); err != nil {
		t.Fatalf("initial Load: %v", err)
	}

	// Next HTTP call from Watch baseline should fail once, then succeed.
	mu.Lock()
	failOnce = true
	body = `{"server":{"port":8080}}`
	mu.Unlock()

	reloads := make(chan *httpPollConfig, 4)
	loader.OnReload(func(old, new *httpPollConfig) { reloads <- new })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := loader.Watch(ctx); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer loader.Stop()

	// After transient failure, watch should still be alive; change content.
	time.Sleep(200 * time.Millisecond)
	mu.Lock()
	body = `{"server":{"port":4242}}`
	mu.Unlock()

	select {
	case cfg := <-reloads:
		if cfg.Server.Port != 4242 {
			t.Errorf("Port = %d, want 4242", cfg.Server.Port)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("watch did not recover and pick up change after transient error")
	}
}

func TestRemoteHTTP_Watch_CancelStopsCleanly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"a":1}`))
	}))
	defer srv.Close()

	src := &RemoteHTTPSource{URL: srv.URL, PollInterval: 30 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())

	changed, stop, err := src.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	time.Sleep(80 * time.Millisecond) // let poll goroutine run
	cancel()
	_ = stop()

	// Project has no dedicated leak detector; Watch closes `changed` on exit
	// (same discipline as YAMLFile/JSONFile). NumGoroutine is too noisy under
	// -race + httptest to assert a hard delta.
	closed := make(chan struct{})
	go func() {
		for range changed {
		}
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("changed channel did not close after cancel")
	}

	// A second receive must be the closed-channel zero value.
	select {
	case _, ok := <-changed:
		if ok {
			t.Fatal("changed still delivering values after close")
		}
	default:
		_, ok := <-changed
		if ok {
			t.Fatal("changed still open")
		}
	}
}

func TestRemoteHTTP_Name(t *testing.T) {
	src := NewRemoteHTTPSource("http://example.internal/config")
	want := "http:http://example.internal/config"
	if src.Name() != want {
		t.Errorf("Name() = %q, want %q", src.Name(), want)
	}
}
