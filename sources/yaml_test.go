package sources

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	defer func() { _ = loader.Stop() }()

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
	defer func() { _ = loader.Stop() }()

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

func TestYAMLFile_EnvSourceOverride(t *testing.T) {
	path := writeYAML(t, "server:\n  port: 8080\nlog:\n  level: info\n")
	t.Setenv("APP_SERVER_PORT", "9090")

	loader := typecfg.New[testConfig](NewYAMLFile(path), typecfg.NewEnvSource("APP"))
	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Port = %d, want 9090 (env should override yaml)", cfg.Server.Port)
	}
}

func TestYAMLFile_StringMapNested(t *testing.T) {
	type labelsConfig struct {
		Labels map[string]string `cfg:"labels"`
	}
	path := writeYAML(t, "labels:\n  env: prod\n  team: backend\n")
	loader := typecfg.New[labelsConfig](NewYAMLFile(path))
	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Labels["env"] != "prod" || cfg.Labels["team"] != "backend" {
		t.Errorf("Labels = %v, want env=prod team=backend", cfg.Labels)
	}
}

func TestYAMLFile_IntSliceFlowStyle(t *testing.T) {
	type portsConfig struct {
		Ports []int `cfg:"ports"`
	}
	path := writeYAML(t, "ports: [8080, 8443]\n")
	loader := typecfg.New[portsConfig](NewYAMLFile(path))
	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Ports) != 2 || cfg.Ports[0] != 8080 || cfg.Ports[1] != 8443 {
		t.Errorf("Ports = %v, want [8080 8443]", cfg.Ports)
	}
}

func TestYAMLFile_IntSliceBlockStyle(t *testing.T) {
	type portsConfig struct {
		Ports []int `cfg:"ports"`
	}
	path := writeYAML(t, "ports:\n  - 8080\n  - 8443\n")
	loader := typecfg.New[portsConfig](NewYAMLFile(path))
	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Ports) != 2 || cfg.Ports[0] != 8080 || cfg.Ports[1] != 8443 {
		t.Errorf("Ports = %v, want [8080 8443]", cfg.Ports)
	}
}

func TestYAMLFile_StringSliceNativeList(t *testing.T) {
	type namesConfig struct {
		Names []string `cfg:"names"`
	}
	path := writeYAML(t, "names:\n  - api\n  - worker\n")
	loader := typecfg.New[namesConfig](NewYAMLFile(path))
	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Names) != 2 || cfg.Names[0] != "api" || cfg.Names[1] != "worker" {
		t.Errorf("Names = %v, want [api worker]", cfg.Names)
	}
}

func TestYAMLFile_IntSliceNativeListInvalidElement(t *testing.T) {
	type portsConfig struct {
		Ports []int `cfg:"ports"`
	}
	path := writeYAML(t, "ports: [8080, bad, 8443]\n")
	loader := typecfg.New[portsConfig](NewYAMLFile(path))
	_, err := loader.Load(context.Background())
	if err == nil {
		t.Fatal("expected parse error")
	}
	var verr *typecfg.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	reason := verr.Errors[0].Reason
	t.Logf("reason: %s", reason)
	if !strings.Contains(reason, `element 1 ("bad")`) {
		t.Errorf("Reason = %q, want index 1 bad element", reason)
	}
}
