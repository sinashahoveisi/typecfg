package jsonschema_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sinashahoveisi/typecfg"
	"github.com/sinashahoveisi/typecfg/jsonschema"
)

type mapSource struct {
	name string
	data map[string]any
}

func (m mapSource) Name() string { return m.name }

func (m mapSource) Load(_ context.Context) (map[string]any, error) {
	return m.data, nil
}

// watchableMap is an in-memory Watchable Source for reload tests.
type watchableMap struct {
	name string
	mu   sync.Mutex
	data map[string]any
	ch   chan struct{}
}

func newWatchableMap(name string, data map[string]any) *watchableMap {
	return &watchableMap{
		name: name,
		data: data,
		ch:   make(chan struct{}, 1),
	}
}

func (w *watchableMap) Name() string { return w.name }

func (w *watchableMap) Load(_ context.Context) (map[string]any, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make(map[string]any, len(w.data))
	for k, v := range w.data {
		out[k] = v
	}
	return out, nil
}

func (w *watchableMap) Watch(ctx context.Context) (<-chan struct{}, func() error, error) {
	stop := func() error { return nil }
	return w.ch, stop, nil
}

func (w *watchableMap) update(data map[string]any) {
	w.mu.Lock()
	w.data = data
	w.mu.Unlock()
	select {
	case w.ch <- struct{}{}:
	default:
	}
}

type serverConfig struct {
	Server struct {
		Port int `cfg:"port"`
	} `cfg:"server"`
}

const serverPortSchema = `{
  "type": "object",
  "required": ["server"],
  "properties": {
    "server": {
      "type": "object",
      "required": ["port"],
      "properties": {
        "port": {
          "type": "number",
          "minimum": 1
        }
      }
    }
  }
}`

func TestValidator_Pass(t *testing.T) {
	fn, err := jsonschema.Validator([]byte(serverPortSchema))
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	loader := typecfg.New[serverConfig](mapSource{
		name: "t",
		data: map[string]any{
			"server": map[string]any{"port": 8080},
		},
	})
	loader.SetRawValidator(fn)
	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Server.Port)
	}
}

func TestValidator_Fail(t *testing.T) {
	fn, err := jsonschema.Validator([]byte(serverPortSchema))
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	loader := typecfg.New[serverConfig](mapSource{
		name: "t",
		data: map[string]any{
			"server": map[string]any{"port": 0},
		},
	})
	loader.SetRawValidator(fn)
	_, err = loader.Load(context.Background())
	if err == nil {
		t.Fatal("expected schema validation error")
	}
	var serr *typecfg.SchemaError
	if !errors.As(err, &serr) {
		t.Fatalf("expected *SchemaError, got %T: %v", err, err)
	}
	msg := err.Error()
	t.Logf("error: %s", msg)
	if !strings.Contains(msg, "schema validation failed") {
		t.Errorf("message should mention schema validation, got %q", msg)
	}
	// Underlying library detail about minimum should surface.
	if !strings.Contains(msg, "minimum") && !strings.Contains(strings.ToLower(msg), "port") {
		t.Errorf("message should include schema violation detail, got %q", msg)
	}
}

func TestValidator_HotReloadPreservesPrevious(t *testing.T) {
	fn, err := jsonschema.Validator([]byte(serverPortSchema))
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}

	src := newWatchableMap("watchable", map[string]any{
		"server": map[string]any{"port": 8080},
	})
	loader := typecfg.New[serverConfig](src)
	loader.SetRawValidator(fn)

	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("initial load failed: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Fatalf("initial Port = %d, want 8080", cfg.Server.Port)
	}

	errCh := make(chan error, 1)
	loader.OnError(func(e error) { errCh <- e })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := loader.Watch(ctx); err != nil {
		t.Fatalf("watch failed: %v", err)
	}
	defer func() { _ = loader.Stop() }()

	// Schema-violating update (port below minimum).
	src.update(map[string]any{
		"server": map[string]any{"port": 0},
	})

	select {
	case e := <-errCh:
		t.Logf("OnError: %s", e.Error())
		if !strings.Contains(e.Error(), "schema validation failed") {
			t.Errorf("OnError message missing schema failure wrapper: %v", e)
		}
		if !strings.Contains(e.Error(), "minimum") {
			t.Errorf("OnError message missing schema detail: %v", e)
		}
		var serr *typecfg.SchemaError
		if !errors.As(e, &serr) {
			t.Errorf("expected *SchemaError, got %T", e)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for OnError after schema-violating reload")
	}

	got := loader.Get()
	if got == nil {
		t.Fatal("Get() returned nil after failed reload; previous config must be kept")
	}
	if got.Server.Port != 8080 {
		t.Errorf("Get().Port = %d, want 8080 (original config)", got.Server.Port)
	}
}
