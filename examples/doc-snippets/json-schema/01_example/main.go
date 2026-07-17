// Snippet from docs/json-schema.md "Example".
package main

import (
	"context"
	"log"

	"github.com/sinashahoveisi/typecfg"
	"github.com/sinashahoveisi/typecfg/jsonschema"
	"github.com/sinashahoveisi/typecfg/sources"
)

type Config struct {
	Server struct {
		Port int `cfg:"port" validate:"required"`
	} `cfg:"server"`
}

func main() {
	schema := []byte(`{
		"type": "object",
		"required": ["server"],
		"properties": {
			"server": {
				"type": "object",
				"required": ["port"],
				"properties": {
					"port": { "type": "number", "minimum": 1 }
				}
			}
		}
	}`)

	fn, err := jsonschema.Validator(schema)
	if err != nil {
		log.Fatal(err)
	}

	loader := typecfg.New[Config](sources.NewYAMLFile("config.yaml"))
	loader.SetRawValidator(fn)

	cfg, err := loader.Load(context.Background())
	if err != nil {
		log.Fatal(err) // may be *typecfg.SchemaError
	}
	log.Printf("port=%d", cfg.Server.Port)
}
