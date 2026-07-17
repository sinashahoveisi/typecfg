// Snippet from README.md "Quick start" — surrounded with package main,
// Config, and ctx so it builds.
package main

import (
	"context"

	"github.com/sinashahoveisi/typecfg"
	"github.com/sinashahoveisi/typecfg/sources"
)

type Config struct {
	Server struct {
		Port int `cfg:"port" default:"8080"`
	} `cfg:"server"`
}

func main() {
	ctx := context.Background()
	loader := typecfg.New[Config](
		sources.NewYAMLFile("config.yaml"),
		typecfg.NewEnvSource("APP"),
	)
	cfg, err := loader.Load(ctx)
	_ = cfg
	_ = err
}
