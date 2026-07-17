// Snippet from docs/guide.md "Hot reload".
package main

import (
	"context"
	"log"

	"github.com/sinashahoveisi/typecfg"
	"github.com/sinashahoveisi/typecfg/sources"
)

type Config struct {
	Log struct {
		Level string `cfg:"level" default:"info" validate:"oneof=debug info warn error"`
	} `cfg:"log"`
}

func main() {
	ctx := context.Background()
	loader := typecfg.New[Config](sources.NewYAMLFile("config.yaml"))

	loader.OnReload(func(old, new *Config) {
		log.Printf("log level changed: %s -> %s", old.Log.Level, new.Log.Level)
	})
	loader.OnError(func(err error) {
		log.Printf("bad config edit ignored: %v", err) // previous config still active
	})
	loader.Watch(ctx)
	defer loader.Stop()
}
