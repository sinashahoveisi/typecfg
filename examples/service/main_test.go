package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sinashahoveisi/typecfg"
	"github.com/sinashahoveisi/typecfg/sources"
)

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

const baseYAML = `server:
  host: 0.0.0.0
  port: 8080
api_key: "dev-only-change-me"
log_level: info
features:
  - metrics
`

const reloadYAML = `server:
  host: 0.0.0.0
  port: 9090
api_key: "dev-only-change-me"
log_level: debug
features:
  - metrics
  - tracing
`

const badYAML = `server:
  host: 0.0.0.0
  port: 0
api_key: "dev-only-change-me"
log_level: info
features:
  - metrics
`

// TestServiceExample_LoadEnvWatchReload exercises the same public API
// path main() documents: YAML + env, Watch, OnReload, OnError, Get keeps
// last good config after a rejected edit.
func TestServiceExample_LoadEnvWatchReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(baseYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SVC_SERVER_HOST", "127.0.0.1")

	loader := typecfg.New[Config](
		sources.NewYAMLFile(path),
		typecfg.NewEnvSource("SVC"),
	)

	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Fatalf("env Host = %q, want 127.0.0.1", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Fatalf("Port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.APIKey != "dev-only-change-me" {
		t.Fatalf("APIKey not loaded")
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want info", cfg.LogLevel)
	}

	reloads := make(chan *Config, 1)
	errs := make(chan error, 1)
	loader.OnReload(func(_, newCfg *Config) { reloads <- newCfg })
	loader.OnError(func(e error) { errs <- e })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := loader.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = loader.Stop() }()

	atomicWrite(t, path, reloadYAML)
	select {
	case got := <-reloads:
		if got.Server.Port != 9090 {
			t.Fatalf("reload Port = %d, want 9090", got.Server.Port)
		}
		if got.LogLevel != "debug" {
			t.Fatalf("reload LogLevel = %q, want debug", got.LogLevel)
		}
		if got.Server.Host != "127.0.0.1" {
			t.Fatalf("env Host must still apply after reload, got %q", got.Server.Host)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for OnReload")
	}

	atomicWrite(t, path, badYAML)
	select {
	case <-errs:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for OnError on invalid port")
	}
	cur := loader.Get()
	if cur == nil || cur.Server.Port != 9090 {
		t.Fatalf("Get after bad edit must keep port 9090, got %+v", cur)
	}
}
