# JSON Schema validation (opt-in)

JSON Schema checks live in a **separate module** so the core
`typecfg` package stays dependency-free. Wire them through
`Loader.SetRawValidator`.

## Install

```bash
go get github.com/sinashahoveisi/typecfg
go get github.com/sinashahoveisi/typecfg/jsonschema
```

## How it works

1. Compile a schema once with `jsonschema.Validator(schemaJSON)`.
2. Pass the returned function to `loader.SetRawValidator(...)`.
3. On each `Load`, after sources are merged and **before** bind/validate,
   the hook runs against the raw `map[string]any`.
4. On failure, `Load` returns a `*typecfg.SchemaError` wrapping the
   library’s validation error.

If you never call `SetRawValidator`, behavior is unchanged.

## Example

```go
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
```

This submodule uses [santhosh-tekuri/jsonschema](https://github.com/santhosh-tekuri/jsonschema)
(v5) — a widely used, stdlib-adjacent validator with broad draft support.
