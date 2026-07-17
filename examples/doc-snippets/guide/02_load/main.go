// Snippet from docs/guide.md "Load from YAML + environment".
package main

import (
	"context"
	"log"

	"github.com/sinashahoveisi/typecfg"
	"github.com/sinashahoveisi/typecfg/sources"
)

type Config struct {
	Server struct {
		Host string `cfg:"host" default:"0.0.0.0"`
		Port int    `cfg:"port" validate:"required,min=1,max=65535"`
	} `cfg:"server"`
}

func main() {
	ctx := context.Background()
	loader := typecfg.New[Config](
		sources.NewYAMLFile("config.yaml"),
		typecfg.NewEnvSource("APP"), // APP_SERVER_PORT overrides server.port
	)

	cfg, err := loader.Load(ctx)
	if err != nil {
		log.Fatal(err)
	}
	_ = cfg
}
