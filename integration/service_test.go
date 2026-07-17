package integration

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sinashahoveisi/typecfg"
	"github.com/sinashahoveisi/typecfg/jsonschema"
	typecfgotel "github.com/sinashahoveisi/typecfg/otel"
	"github.com/sinashahoveisi/typecfg/sources"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// secretValue is planted in the YAML api_key field. Failure-path assertions
// require that this exact string never appears in SchemaError, OnError, or
// slog output.
const secretValue = "super-secret-integration-key-DO-NOT-LEAK"

const serviceSchema = `{
  "type": "object",
  "required": ["server", "api_key"],
  "properties": {
    "server": {
      "type": "object",
      "required": ["port"],
      "properties": {
        "port": { "type": "number", "minimum": 1, "maximum": 65535 },
        "host": { "type": "string" }
      }
    },
    "api_key": { "type": "string", "minLength": 1 },
    "log_level": { "type": "string" },
    "features": { "type": "array", "items": { "type": "string" } },
    "labels": { "type": "object" },
    "started_at": { "type": "string" }
  }
}`

const baseYAML = `server:
  host: 0.0.0.0
  port: 8080
features:
  - metrics
  - tracing
labels:
  env: test
  team: platform
started_at: "2026-01-15T12:00:00Z"
api_key: "super-secret-integration-key-DO-NOT-LEAK"
log_level: info
`

// reloadYAML keeps the secret and env-overridden host; changes port + features
// + log_level so Watch/Get can observe a real file-driven update.
const reloadYAML = `server:
  host: 0.0.0.0
  port: 9090
features:
  - metrics
  - tracing
  - profiling
labels:
  env: test
  team: platform
started_at: "2026-01-15T12:00:00Z"
api_key: "super-secret-integration-key-DO-NOT-LEAK"
log_level: debug
`

// badSchemaYAML uses a string port so jsonschema (type: number) fails before bind.
const badSchemaYAML = `server:
  host: 0.0.0.0
  port: "not-a-number"
features:
  - metrics
labels:
  env: test
started_at: "2026-01-15T12:00:00Z"
api_key: "super-secret-integration-key-DO-NOT-LEAK"
log_level: info
`

type memLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (m *memLog) Enabled(context.Context, slog.Level) bool { return true }

func (m *memLog) Handle(_ context.Context, r slog.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buf.WriteString(r.Level.String())
	m.buf.WriteByte(' ')
	m.buf.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		m.buf.WriteByte(' ')
		m.buf.WriteString(a.String())
		return true
	})
	m.buf.WriteByte('\n')
	return nil
}

func (m *memLog) WithAttrs([]slog.Attr) slog.Handler { return m }
func (m *memLog) WithGroup(string) slog.Handler      { return m }

func (m *memLog) String() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.buf.String()
}

func (m *memLog) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buf.Reset()
}

func counterSum(t *testing.T, rm *metricdata.ResourceMetrics, name string) int64 {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("%s: unexpected data type %T", name, m.Data)
			}
			var total int64
			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
			return total
		}
	}
	return 0
}

func waitCounter(t *testing.T, reader *metric.ManualReader, name string, want int64, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		var rm metricdata.ResourceMetrics
		if err := reader.Collect(context.Background(), &rm); err != nil {
			t.Fatal(err)
		}
		if counterSum(t, &rm, name) >= want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %s >= %d", name, want)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func atomicWrite(t *testing.T, path, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	tmp := filepath.Join(dir, filepath.Base(path)+".tmp")
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
}

func assertNoSecretLeak(t *testing.T, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if strings.Contains(p, secretValue) {
			t.Fatalf("secret value leaked into output:\n%s", p)
		}
	}
}

func newServiceLoader(t *testing.T, path string, generated bool) *typecfg.Loader[ServiceConfig] {
	t.Helper()
	yamlSrc := sources.NewYAMLFile(path)
	envSrc := typecfg.NewEnvSource("APP")
	var loader *typecfg.Loader[ServiceConfig]
	if generated {
		loader = typecfg.NewGenerated[ServiceConfig](ServiceConfigBinder{}, yamlSrc, envSrc)
	} else {
		loader = typecfg.New[ServiceConfig](yamlSrc, envSrc)
	}
	fn, err := jsonschema.Validator([]byte(serviceSchema))
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	loader.SetRawValidator(fn)
	return loader
}

func TestIntegration_HappyPath_ReflectionAndGenerated(t *testing.T) {
	for _, tc := range []struct {
		name      string
		generated bool
	}{
		{"reflection", false},
		{"generated", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(path, []byte(baseYAML), 0o644); err != nil {
				t.Fatal(err)
			}

			// Env overrides host (string) so schema still sees numeric port from YAML.
			t.Setenv("APP_SERVER_HOST", "127.0.0.1")

			loader := newServiceLoader(t, path, tc.generated)
			logBuf := &memLog{}
			loader.SetLogger(slog.New(logBuf))

			reader := metric.NewManualReader()
			provider := metric.NewMeterProvider(metric.WithReader(reader))
			t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
			if err := typecfgotel.Register(loader, provider.Meter("integration")); err != nil {
				t.Fatal(err)
			}

			cfg, err := loader.Load(context.Background())
			if err != nil {
				t.Fatalf("initial Load: %v", err)
			}
			if cfg.Server.Host != "127.0.0.1" {
				t.Fatalf("env override Host = %q, want 127.0.0.1", cfg.Server.Host)
			}
			if cfg.Server.Port != 8080 {
				t.Fatalf("file Port = %d, want 8080", cfg.Server.Port)
			}
			if cfg.APIKey != secretValue {
				t.Fatalf("APIKey not loaded")
			}
			if !reflect.DeepEqual(cfg.Features, []string{"metrics", "tracing"}) {
				t.Fatalf("Features = %v", cfg.Features)
			}

			reloads := make(chan *ServiceConfig, 1)
			var reloadOnce sync.Once
			loader.OnReload(func(old, newCfg *ServiceConfig) {
				reloadOnce.Do(func() { reloads <- newCfg })
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := loader.Watch(ctx); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = loader.Stop() }()

			logBuf.Reset()
			atomicWrite(t, path, reloadYAML)

			select {
			case got := <-reloads:
				if got.Server.Port != 9090 {
					t.Fatalf("reload Port = %d, want 9090", got.Server.Port)
				}
				if got.LogLevel != "debug" {
					t.Fatalf("reload LogLevel = %q, want debug", got.LogLevel)
				}
				if !reflect.DeepEqual(got.Features, []string{"metrics", "tracing", "profiling"}) {
					t.Fatalf("reload Features = %v", got.Features)
				}
				if got.Server.Host != "127.0.0.1" {
					t.Fatalf("env Host must still apply after reload, got %q", got.Server.Host)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for OnReload")
			}

			cur := loader.Get()
			if cur == nil || cur.Server.Port != 9090 {
				t.Fatalf("Get() after reload = %+v", cur)
			}

			logs := logBuf.String()
			if !strings.Contains(logs, "INFO") || !strings.Contains(logs, "config reloaded") {
				t.Fatalf("SetLogger Info missing; logs:\n%s", logs)
			}
			assertNoSecretLeak(t, logs)

			waitCounter(t, reader, "config_reload_total", 1, 5*time.Second)
			var rm metricdata.ResourceMetrics
			if err := reader.Collect(context.Background(), &rm); err != nil {
				t.Fatal(err)
			}
			if counterSum(t, &rm, "config_reload_errors_total") != 0 {
				t.Fatalf("errors_total = %d, want 0", counterSum(t, &rm, "config_reload_errors_total"))
			}
		})
	}
}

func TestIntegration_SchemaFailure_KeepsPreviousAndHidesSecret(t *testing.T) {
	for _, tc := range []struct {
		name      string
		generated bool
	}{
		{"reflection", false},
		{"generated", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(path, []byte(baseYAML), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Setenv("APP_SERVER_HOST", "127.0.0.1")

			loader := newServiceLoader(t, path, tc.generated)
			logBuf := &memLog{}
			loader.SetLogger(slog.New(logBuf))

			reader := metric.NewManualReader()
			provider := metric.NewMeterProvider(metric.WithReader(reader))
			t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
			if err := typecfgotel.Register(loader, provider.Meter("integration")); err != nil {
				t.Fatal(err)
			}

			origin, err := loader.Load(context.Background())
			if err != nil {
				t.Fatalf("initial Load: %v", err)
			}
			originPort := origin.Server.Port

			errs := make(chan error, 1)
			var errOnce sync.Once
			loader.OnError(func(e error) {
				errOnce.Do(func() { errs <- e })
			})
			loader.OnReload(func(old, newCfg *ServiceConfig) {
				t.Errorf("OnReload must not fire on schema failure")
			})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := loader.Watch(ctx); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = loader.Stop() }()

			logBuf.Reset()
			atomicWrite(t, path, badSchemaYAML)

			var failErr error
			select {
			case failErr = <-errs:
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for OnError")
			}

			var serr *typecfg.SchemaError
			if !errors.As(failErr, &serr) {
				t.Fatalf("want *SchemaError, got %T: %v", failErr, failErr)
			}
			// Secret must not appear in any failure-path text:
			// 1) OnError / SchemaError message
			// 2) underlying jsonschema error (Unwrap)
			// 3) full slog capture from SetLogger
			// FieldError text is not produced on this path (schema runs before
			// bind/validate); assert that explicitly so a future regression
			// that somehow surfaces ValidationError cannot skip the check.
			var verr *typecfg.ValidationError
			if errors.As(failErr, &verr) {
				t.Fatalf("schema failure must not also be *ValidationError: %+v", verr)
			}
			underlying := ""
			if u := serr.Unwrap(); u != nil {
				underlying = u.Error()
			}
			assertNoSecretLeak(t,
				failErr.Error(), // 1a: OnError value as returned to callbacks
				serr.Error(),    // 1b: *SchemaError.Error()
				underlying,      // 1c: wrapped jsonschema message
			)

			got := loader.Get()
			if got == nil || got.Server.Port != originPort {
				t.Fatalf("Get() must keep origin port %d, got %+v", originPort, got)
			}
			if got.APIKey != secretValue {
				t.Fatalf("origin secret must remain in Get()")
			}

			logs := logBuf.String()
			if !strings.Contains(logs, "ERROR") || !strings.Contains(logs, "config reload failed") {
				t.Fatalf("SetLogger Error missing; logs:\n%s", logs)
			}
			assertNoSecretLeak(t, logs) // 2: full slog buffer (includes err attr)

			waitCounter(t, reader, "config_reload_errors_total", 1, 5*time.Second)
			var rm metricdata.ResourceMetrics
			if err := reader.Collect(context.Background(), &rm); err != nil {
				t.Fatal(err)
			}
			if counterSum(t, &rm, "config_reload_total") != 0 {
				t.Fatalf("reload_total = %d, want 0 on failure", counterSum(t, &rm, "config_reload_total"))
			}
		})
	}
}

func TestIntegration_ReflectionMatchesGenerated_FinalState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(baseYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_SERVER_HOST", "127.0.0.1")

	run := func(generated bool) *ServiceConfig {
		t.Helper()
		loader := newServiceLoader(t, path, generated)
		cfg, err := loader.Load(context.Background())
		if err != nil {
			t.Fatalf("generated=%v Load: %v", generated, err)
		}
		if err := loader.Stop(); err != nil {
			t.Fatal(err)
		}
		return cfg
	}

	ref := run(false)
	gen := run(true)
	if ref.Server != gen.Server || ref.LogLevel != gen.LogLevel || ref.APIKey != gen.APIKey {
		t.Fatalf("mismatch scalars\nref=%+v\ngen=%+v", ref, gen)
	}
	if !ref.StartedAt.Equal(gen.StartedAt) {
		t.Fatalf("StartedAt mismatch")
	}
	if !reflect.DeepEqual(ref.Features, gen.Features) || !reflect.DeepEqual(ref.Labels, gen.Labels) {
		t.Fatalf("mismatch composites\nref features=%v labels=%v\ngen features=%v labels=%v",
			ref.Features, ref.Labels, gen.Features, gen.Labels)
	}
}
