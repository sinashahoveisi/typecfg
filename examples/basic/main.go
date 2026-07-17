package main

import (
	"context"
	"log"
	"time"

	"github.com/sinashahoveisi/typecfg"
	"github.com/sinashahoveisi/typecfg/sources"
)

type Config struct {
	Server struct {
		Host string `cfg:"host" default:"0.0.0.0"`
		Port int    `cfg:"port" validate:"required,min=1,max=65535"`
	} `cfg:"server"`

	Log struct {
		Level string `cfg:"level" default:"info" validate:"oneof=debug info warn error"`
	} `cfg:"log"`

	ShutdownTimeout time.Duration `cfg:"shutdown_timeout" default:"5s"`
}

func main() {
	ctx := context.Background()

	loader := typecfg.New[Config](
		sources.NewYAMLFile("config.yaml"),
		typecfg.NewEnvSource("APP"), // APP_SERVER_PORT overrides server.port
	)

	cfg, err := loader.Load(ctx)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}
	log.Printf("starting on %s:%d (log level: %s)", cfg.Server.Host, cfg.Server.Port, cfg.Log.Level)

	loader.OnReload(func(old, new *Config) {
		log.Printf("config reloaded: log level %s -> %s", old.Log.Level, new.Log.Level)
	})
	loader.OnError(func(err error) {
		log.Printf("config reload failed, keeping previous config: %v", err)
	})

	if err := loader.Watch(ctx); err != nil {
		log.Fatalf("failed to watch config: %v", err)
	}
	defer func() { _ = loader.Stop() }()

	select {} // run forever; edit config.yaml to see hot reload in action
}
