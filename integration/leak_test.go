package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sinashahoveisi/typecfg"
	"github.com/sinashahoveisi/typecfg/jsonschema"
	"github.com/sinashahoveisi/typecfg/sources"
)

// Goroutine-count settle policy for leak checks:
//
//   - Poll every 50ms for up to 2s after Stop()/server Close — watcher and
//     ticker goroutines do not always exit in the same scheduler tick as
//     cancel().
//   - Tolerance ±2 around the pre-test baseline absorbs transient GC /
//     finalizer / testing-runtime goroutines. Verified stable across five
//     consecutive `go test -race -count=5` runs of this file.
const (
	goroutineSettleTimeout = 2 * time.Second
	goroutinePollInterval  = 50 * time.Millisecond
	goroutineTolerance     = 2
)

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func baselineGoroutines() int {
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	return runtime.NumGoroutine()
}

func waitGoroutinesNear(t *testing.T, baseline int) {
	t.Helper()
	deadline := time.Now().Add(goroutineSettleTimeout)
	var last int
	for {
		last = runtime.NumGoroutine()
		if absInt(last-baseline) <= goroutineTolerance {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("goroutine count did not return near baseline %d (±%d) within %s; last=%d",
				baseline, goroutineTolerance, goroutineSettleTimeout, last)
		}
		time.Sleep(goroutinePollInterval)
	}
}

func checkValidServiceConfig(cfg *ServiceConfig) error {
	if cfg == nil {
		return fmt.Errorf("config is nil (partial/half-written Get/Load result)")
	}
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("invalid Port %d", cfg.Server.Port)
	}
	if cfg.Server.Host == "" {
		return fmt.Errorf("empty Host")
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("empty APIKey")
	}
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid LogLevel %q", cfg.LogLevel)
	}
	return nil
}

func assertValidServiceConfig(t *testing.T, cfg *ServiceConfig) {
	t.Helper()
	if err := checkValidServiceConfig(cfg); err != nil {
		t.Fatal(err)
	}
}

func serviceJSON(port int, logLevel string) string {
	return fmt.Sprintf(`{
  "server": {"host": "127.0.0.1", "port": %d},
  "features": ["metrics"],
  "labels": {"env": "test"},
  "started_at": "2026-01-15T12:00:00Z",
  "api_key": %q,
  "log_level": %q
}`, port, secretValue, logLevel)
}

func serviceYAML(port int, logLevel string) string {
	return fmt.Sprintf(`server:
  host: 127.0.0.1
  port: %d
features:
  - metrics
labels:
  env: test
started_at: "2026-01-15T12:00:00Z"
api_key: %q
log_level: %s
`, port, secretValue, logLevel)
}

// --- goroutine leak: YAMLFile + EnvSource (fsnotify) ---

func TestLeak_YAMLFileWatch_StopReturnsToBaseline(t *testing.T) {
	baseline := baselineGoroutines()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(serviceYAML(8080, "info")), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_SERVER_HOST", "127.0.0.1")

	loader := newServiceLoader(t, path, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reloads := make(chan struct{}, 8)
	errs := make(chan struct{}, 8)
	loader.OnReload(func(_, _ *ServiceConfig) { reloads <- struct{}{} })
	loader.OnError(func(error) { errs <- struct{}{} })

	if _, err := loader.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := loader.Watch(ctx); err != nil {
		t.Fatal(err)
	}

	// Valid reload → OnReload path.
	atomicWrite(t, path, serviceYAML(9090, "debug"))
	select {
	case <-reloads:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for valid OnReload")
	}

	// Invalid (schema) reload → OnError path.
	atomicWrite(t, path, badSchemaYAML)
	select {
	case <-errs:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for OnError")
	}

	// Another valid reload after failure.
	atomicWrite(t, path, serviceYAML(8081, "info"))
	select {
	case <-reloads:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for second OnReload")
	}

	if err := loader.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	cancel()
	waitGoroutinesNear(t, baseline)
}

// --- goroutine leak: RemoteHTTPSource (ticker poll) ---

func TestLeak_RemoteHTTPWatch_StopReturnsToBaseline(t *testing.T) {
	baseline := baselineGoroutines()

	var body atomic.Value
	body.Store(serviceJSON(8080, "info"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body.Load().(string)))
	}))
	defer srv.Close()

	src := &sources.RemoteHTTPSource{
		URL:          srv.URL,
		Format:       "json",
		PollInterval: 40 * time.Millisecond,
	}
	loader := typecfg.New[ServiceConfig](src)
	fn, err := jsonschema.Validator([]byte(serviceSchema))
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	loader.SetRawValidator(fn)

	reloads := make(chan struct{}, 8)
	errs := make(chan struct{}, 8)
	loader.OnReload(func(_, _ *ServiceConfig) { reloads <- struct{}{} })
	loader.OnError(func(error) { errs <- struct{}{} })

	if _, err := loader.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := loader.Watch(ctx); err != nil {
		t.Fatal(err)
	}

	body.Store(serviceJSON(9090, "debug"))
	select {
	case <-reloads:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for HTTP OnReload")
	}

	// Schema-invalid body → OnError; Get keeps previous.
	body.Store(`{"server":{"host":"127.0.0.1","port":"not-a-number"},"api_key":"x","log_level":"info","started_at":"2026-01-15T12:00:00Z"}`)
	select {
	case <-errs:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for HTTP OnError")
	}

	body.Store(serviceJSON(8081, "warn"))
	select {
	case <-reloads:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for second HTTP OnReload")
	}

	if err := loader.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	cancel()
	srv.Close()
	waitGoroutinesNear(t, baseline)
}

// --- sequential create/Stop cycles catch slow leaks ---

func TestLeak_SequentialLoaderCycles_NoSlowLeak(t *testing.T) {
	baseline := baselineGoroutines()

	const cycles = 10
	for i := 0; i < cycles; i++ {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(path, []byte(serviceYAML(8080+i, "info")), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("APP_SERVER_HOST", "127.0.0.1")

		loader := newServiceLoader(t, path, false)
		if _, err := loader.Load(context.Background()); err != nil {
			t.Fatalf("cycle %d Load: %v", i, err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		if err := loader.Watch(ctx); err != nil {
			cancel()
			t.Fatal(err)
		}
		atomicWrite(t, path, serviceYAML(9000+i, "debug"))
		// Brief window for the reload goroutine to run; we do not require
		// observing OnReload — the point is Watch/Stop lifecycle churn.
		time.Sleep(80 * time.Millisecond)
		if err := loader.Stop(); err != nil {
			cancel()
			t.Fatalf("cycle %d Stop: %v", i, err)
		}
		cancel()
	}

	waitGoroutinesNear(t, baseline)
}

// --- concurrent Get during hot-reload ---

func TestConcurrent_GetDuringHotReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(serviceYAML(8080, "info")), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_SERVER_HOST", "127.0.0.1")

	loader := newServiceLoader(t, path, false)
	if _, err := loader.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := loader.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = loader.Stop() }()

	var reloadN atomic.Int64
	var callbackErr atomic.Value
	loader.OnReload(func(_, newCfg *ServiceConfig) {
		if err := checkValidServiceConfig(newCfg); err != nil {
			callbackErr.Store(err)
			return
		}
		reloadN.Add(1)
	})

	const readers = 32
	const duration = 800 * time.Millisecond
	done := make(chan struct{})
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(readers)
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					if err := checkValidServiceConfig(loader.Get()); err != nil {
						select {
						case errCh <- err:
						default:
						}
						return
					}
				}
			}
		}()
	}

	deadline := time.Now().Add(duration)
	port := 8100
	for time.Now().Before(deadline) {
		port++
		atomicWrite(t, path, serviceYAML(port, "info"))
		time.Sleep(30 * time.Millisecond)
	}
	close(done)
	wg.Wait()
	select {
	case err := <-errCh:
		t.Fatal(err)
	default:
	}
	if v := callbackErr.Load(); v != nil {
		t.Fatal(v.(error))
	}

	if reloadN.Load() < 1 {
		t.Fatal("expected at least one successful hot-reload during concurrent Gets")
	}
	assertValidServiceConfig(t, loader.Get())
}

// --- concurrent OnReload/OnError registration while reloads fire ---
//
// Invariant under check (also enforced by -race):
//   OnReload/OnError append under Loader.mu.Lock; reload snapshots the
//   callback slices under RLock. Concurrent registration while callbacks
//   fire must not race, panic, or double-fire a callback that was already
//   snapshotted for a given reload. Concrete check: a single callback
//   registered before Watch fires exactly once per successful/failed
//   reload we trigger (no miss, no double), while other goroutines keep
//   appending empty callbacks.

func TestConcurrent_CallbackRegistrationDuringReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(serviceYAML(8080, "info")), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_SERVER_HOST", "127.0.0.1")

	loader := newServiceLoader(t, path, false)
	if _, err := loader.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}

	var successHits atomic.Int64
	var errorHits atomic.Int64
	var callbackErr atomic.Value
	loader.OnReload(func(old, newCfg *ServiceConfig) {
		if err := checkValidServiceConfig(old); err != nil {
			callbackErr.Store(err)
			return
		}
		if err := checkValidServiceConfig(newCfg); err != nil {
			callbackErr.Store(err)
			return
		}
		successHits.Add(1)
	})
	loader.OnError(func(err error) {
		if err == nil {
			callbackErr.Store(fmt.Errorf("OnError called with nil"))
			return
		}
		errorHits.Add(1)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := loader.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = loader.Stop() }()

	const registrars = 8
	regDone := make(chan struct{})
	var regWG sync.WaitGroup
	regWG.Add(registrars)
	for i := 0; i < registrars; i++ {
		go func() {
			defer regWG.Done()
			for {
				select {
				case <-regDone:
					return
				default:
					loader.OnReload(func(_, _ *ServiceConfig) {})
					loader.OnError(func(error) {})
					runtime.Gosched()
				}
			}
		}()
	}

	const wantSuccess = 6
	const wantErrors = 4
	for i := 0; i < wantSuccess; i++ {
		atomicWrite(t, path, serviceYAML(8200+i, "debug"))
		waitAtomicAtLeast(t, &successHits, int64(i+1), 5*time.Second)
	}
	for i := 0; i < wantErrors; i++ {
		atomicWrite(t, path, badSchemaYAML)
		waitAtomicAtLeast(t, &errorHits, int64(i+1), 5*time.Second)
		// Restore a valid config so the next bad write is a distinct reload.
		atomicWrite(t, path, serviceYAML(8300+i, "info"))
		waitAtomicAtLeast(t, &successHits, int64(wantSuccess+i+1), 5*time.Second)
	}

	close(regDone)
	regWG.Wait()

	if v := callbackErr.Load(); v != nil {
		t.Fatal(v.(error))
	}
	if got := successHits.Load(); got != int64(wantSuccess+wantErrors) {
		t.Fatalf("pre-registered OnReload hits = %d, want %d (no miss/double-fire)", got, wantSuccess+wantErrors)
	}
	if got := errorHits.Load(); got != int64(wantErrors) {
		t.Fatalf("pre-registered OnError hits = %d, want %d (no miss/double-fire)", got, wantErrors)
	}
}

func waitAtomicAtLeast(t *testing.T, v *atomic.Int64, want int64, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		if v.Load() >= want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for counter >= %d (have %d)", want, v.Load())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// --- concurrent Load() without Watch ---

func TestConcurrent_LoadSameLoader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(serviceYAML(8080, "info")), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_SERVER_HOST", "127.0.0.1")

	loader := newServiceLoader(t, path, false)

	const callers = 64
	var wg sync.WaitGroup
	wg.Add(callers)
	errCh := make(chan error, callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			cfg, err := loader.Load(context.Background())
			if err != nil {
				errCh <- err
				return
			}
			if cfg == nil || cfg.Server.Port != 8080 || cfg.APIKey != secretValue {
				errCh <- fmt.Errorf("inconsistent Load result: %+v", cfg)
				return
			}
			if err := checkValidServiceConfig(cfg); err != nil {
				errCh <- err
				return
			}
			if err := checkValidServiceConfig(loader.Get()); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}
