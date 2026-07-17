// Command service is a small, realistic typecfg adoption example:
// YAML + env, slog on reload, and Watch that keeps the last good config.
package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"

	"github.com/sinashahoveisi/typecfg"
	"github.com/sinashahoveisi/typecfg/sources"
)

// Config is what a typical service would load once at startup and again
// on file change. It is intentionally modest — not an exhaustive tag demo.
type Config struct {
	Server struct {
		Host string `cfg:"host" default:"0.0.0.0"`
		Port int    `cfg:"port" validate:"required,min=1,max=65535"`
	} `cfg:"server"`

	// APIKey is never printed in FieldError reasons (secret:"true").
	APIKey string `cfg:"api_key" secret:"true" validate:"required,min=8"`

	LogLevel string   `cfg:"log_level" default:"info" validate:"oneof=debug info warn error"`
	Features []string `cfg:"features"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	loader := typecfg.New[Config](
		sources.NewYAMLFile("config.yaml"),
		typecfg.NewEnvSource("SVC"), // e.g. SVC_SERVER_PORT overrides server.port
	)
	loader.SetLogger(slog.Default())

	cfg, err := loader.Load(ctx)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	log.Printf("listening on %s:%d log=%s features=%v",
		cfg.Server.Host, cfg.Server.Port, cfg.LogLevel, cfg.Features)

	loader.OnReload(func(old, newCfg *Config) {
		log.Printf("config reloaded: port %d -> %d, log %s -> %s",
			old.Server.Port, newCfg.Server.Port, old.LogLevel, newCfg.LogLevel)
	})
	loader.OnError(func(err error) {
		// Get() still returns the previous good config.
		log.Printf("reload rejected, keeping previous config: %v", err)
	})

	if err := loader.Watch(ctx); err != nil {
		log.Fatalf("watch config: %v", err)
	}
	defer func() { _ = loader.Stop() }()

	<-ctx.Done()
	log.Printf("shutting down with port=%d", loader.Get().Server.Port)
}
