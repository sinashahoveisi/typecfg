package typecfg

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

func TestDiff_OneScalarChange(t *testing.T) {
	type cfg struct {
		Port int `cfg:"port"`
	}
	old := &cfg{Port: 8080}
	newCfg := &cfg{Port: 9090}
	changes := Diff(old, newCfg)
	if len(changes) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(changes), changes)
	}
	c := changes[0]
	if c.Path != "port" || c.Old != 8080 || c.New != 9090 {
		t.Errorf("got %+v", c)
	}
}

func TestDiff_NoChanges(t *testing.T) {
	type cfg struct {
		Port int `cfg:"port"`
	}
	a := &cfg{Port: 8080}
	b := &cfg{Port: 8080}
	if got := Diff(a, b); len(got) != 0 {
		t.Fatalf("want empty, got %+v", got)
	}
}

func TestDiff_SecretRedacted(t *testing.T) {
	type cfg struct {
		Token string `cfg:"token" secret:"true"`
	}
	old := &cfg{Token: "secret-old"}
	newCfg := &cfg{Token: "secret-new"}
	changes := Diff(old, newCfg)
	if len(changes) != 1 {
		t.Fatalf("len = %d, want 1", len(changes))
	}
	c := changes[0]
	if c.Path != "token" {
		t.Errorf("Path = %q", c.Path)
	}
	if c.Old != redactedMarker || c.New != redactedMarker {
		t.Errorf("Old=%v New=%v, want both %q", c.Old, c.New, redactedMarker)
	}
	if c.Old == "secret-old" || c.New == "secret-new" {
		t.Error("raw secret values must not appear")
	}
}

func TestDiff_NestedDottedPath(t *testing.T) {
	type cfg struct {
		Server struct {
			Port int `cfg:"port"`
		} `cfg:"server"`
	}
	old := &cfg{}
	old.Server.Port = 1
	newCfg := &cfg{}
	newCfg.Server.Port = 2
	changes := Diff(old, newCfg)
	if len(changes) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(changes), changes)
	}
	if changes[0].Path != "server.port" {
		t.Errorf("Path = %q, want server.port", changes[0].Path)
	}
	if changes[0].Old != 1 || changes[0].New != 2 {
		t.Errorf("got %+v", changes[0])
	}
}

func TestDiff_NilOldReportsAllLeaves(t *testing.T) {
	type cfg struct {
		Server struct {
			Port int    `cfg:"port"`
			Host string `cfg:"host"`
		} `cfg:"server"`
		Level string `cfg:"level"`
	}
	newCfg := &cfg{}
	newCfg.Server.Port = 8080
	newCfg.Server.Host = "localhost"
	newCfg.Level = "info"

	changes := Diff[cfg](nil, newCfg)
	if len(changes) != 3 {
		t.Fatalf("len = %d, want 3: %+v", len(changes), changes)
	}
	byPath := map[string]FieldChange{}
	for _, c := range changes {
		byPath[c.Path] = c
		if c.Old != nil {
			t.Errorf("%s: Old = %v, want nil", c.Path, c.Old)
		}
	}
	for _, p := range []string{"server.port", "server.host", "level"} {
		if _, ok := byPath[p]; !ok {
			t.Errorf("missing path %q", p)
		}
	}
}

type loggerWatchable struct {
	mu   sync.Mutex
	data map[string]any
	ch   chan struct{}
}

func newLoggerWatchable(data map[string]any) *loggerWatchable {
	return &loggerWatchable{data: data, ch: make(chan struct{}, 1)}
}

func (w *loggerWatchable) Name() string { return "watchable" }

func (w *loggerWatchable) Load(_ context.Context) (map[string]any, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make(map[string]any, len(w.data))
	for k, v := range w.data {
		out[k] = v
	}
	return out, nil
}

func (w *loggerWatchable) Watch(ctx context.Context) (<-chan struct{}, func() error, error) {
	return w.ch, func() error { return nil }, nil
}

func (w *loggerWatchable) update(data map[string]any) {
	w.mu.Lock()
	w.data = data
	w.mu.Unlock()
}

type logPortConfig struct {
	Port int `cfg:"port" validate:"required,min=1"`
}

func TestSetLogger_SuccessfulReload(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	src := newLoggerWatchable(map[string]any{"port": 8080})
	loader := New[logPortConfig](src)
	if _, err := loader.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	loader.SetLogger(logger)

	src.update(map[string]any{"port": 9090})
	loader.reload(context.Background())

	out := buf.String()
	if !strings.Contains(out, `"msg":"config reloaded"`) && !strings.Contains(out, "config reloaded") {
		t.Fatalf("expected Info reload log, got %s", out)
	}
	if !strings.Contains(out, "port") {
		t.Errorf("expected changed field path in log, got %s", out)
	}
	if !strings.Contains(out, "9090") {
		t.Errorf("expected new value in log, got %s", out)
	}
}

func TestSetLogger_FailedReload(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	src := newLoggerWatchable(map[string]any{"port": 8080})
	loader := New[logPortConfig](src)
	if _, err := loader.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	loader.SetLogger(logger)

	// Invalid: port missing / zero fails validation.
	src.update(map[string]any{})
	loader.reload(context.Background())

	out := buf.String()
	if !strings.Contains(out, "config reload failed") {
		t.Fatalf("expected Error reload log, got %s", out)
	}
	if !strings.Contains(out, `"level":"ERROR"`) && !strings.Contains(out, "ERROR") {
		t.Errorf("expected Error level, got %s", out)
	}
}

func TestSetLogger_NilNoLoggingCallbacksStillFire(t *testing.T) {
	src := newLoggerWatchable(map[string]any{"port": 8080})
	loader := New[logPortConfig](src)
	if _, err := loader.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Do not call SetLogger.

	var reloads int
	var errs int
	loader.OnReload(func(old, new *logPortConfig) { reloads++ })
	loader.OnError(func(error) { errs++ })

	src.update(map[string]any{"port": 9090})
	loader.reload(context.Background())
	if reloads != 1 {
		t.Errorf("OnReload calls = %d, want 1", reloads)
	}

	src.update(map[string]any{})
	loader.reload(context.Background())
	if errs != 1 {
		t.Errorf("OnError calls = %d, want 1", errs)
	}
}
