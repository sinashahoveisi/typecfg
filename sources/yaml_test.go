package sources

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sinashahoveisi/typecfg"
)

type testConfig struct {
	Server struct {
		Host string `cfg:"host" default:"0.0.0.0"`
		Port int    `cfg:"port" validate:"required,min=1,max=65535"`
	} `cfg:"server"`
	Log struct {
		Level string `cfg:"level" default:"info" validate:"oneof=debug info warn error"`
	} `cfg:"log"`
}

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestYAMLFile_Load_Basic(t *testing.T) {
	path := writeYAML(t, "server:\n  port: 8080\nlog:\n  level: debug\n")
	loader := typecfg.New[testConfig](NewYAMLFile(path))

	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Host default not applied, got %q", cfg.Server.Host)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Level = %q, want debug", cfg.Log.Level)
	}
}

func TestYAMLFile_HotReload(t *testing.T) {
	path := writeYAML(t, "server:\n  port: 8080\nlog:\n  level: info\n")
	loader := typecfg.New[testConfig](NewYAMLFile(path))

	if _, err := loader.Load(context.Background()); err != nil {
		t.Fatalf("initial load failed: %v", err)
	}

	reloaded := make(chan *testConfig, 1)
	loader.OnReload(func(old, new *testConfig) { reloaded <- new })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := loader.Watch(ctx); err != nil {
		t.Fatalf("watch failed: %v", err)
	}
	defer loader.Stop()

	if err := os.WriteFile(path, []byte("server:\n  port: 9999\nlog:\n  level: info\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case cfg := <-reloaded:
		if cfg.Server.Port != 9999 {
			t.Errorf("Port after reload = %d, want 9999", cfg.Server.Port)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for hot reload")
	}
}

func TestYAMLFile_HotReloadAtomicReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("server:\n  port: 8080\nlog:\n  level: info\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := typecfg.New[testConfig](NewYAMLFile(path))

	if _, err := loader.Load(context.Background()); err != nil {
		t.Fatalf("initial load failed: %v", err)
	}

	reloaded := make(chan *testConfig, 1)
	loader.OnReload(func(old, new *testConfig) { reloaded <- new })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := loader.Watch(ctx); err != nil {
		t.Fatalf("watch failed: %v", err)
	}
	defer loader.Stop()

	tmp := filepath.Join(dir, "config.yaml.tmp")
	if err := os.WriteFile(tmp, []byte("server:\n  port: 9999\nlog:\n  level: info\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}

	select {
	case cfg := <-reloaded:
		if cfg.Server.Port != 9999 {
			t.Errorf("Port after atomic replace reload = %d, want 9999", cfg.Server.Port)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for hot reload after atomic replace")
	}
}
