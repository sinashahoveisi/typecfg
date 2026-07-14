# Getting started with typecfg

typecfg loads a typed Go struct from one or more config sources, validates
it in one pass, and optionally hot-reloads when a watched source changes.

## Install

The core module has zero third-party dependencies:

```bash
go get github.com/sinashahoveisi/typecfg
```

File sources (YAML/JSON with hot reload) live in a separate module:

```bash
go get github.com/sinashahoveisi/typecfg/sources
```

## Define your config struct

Use `cfg` tags for key names, `default` for fallbacks, and `validate` for
rules:

```go
type Config struct {
	Server struct {
		Host string `cfg:"host" default:"0.0.0.0"`
		Port int    `cfg:"port" validate:"required,min=1,max=65535"`
	} `cfg:"server"`
	Log struct {
		Level string `cfg:"level" default:"info" validate:"oneof=debug info warn error"`
	} `cfg:"log"`
}
```

## Load from YAML + environment

Sources are merged in order; later sources override earlier ones. The
usual pattern is a base file plus env vars for container deployments:

```go
import (
	"github.com/sinashahoveisi/typecfg"
	"github.com/sinashahoveisi/typecfg/sources"
)

loader := typecfg.New[Config](
	sources.NewYAMLFile("config.yaml"),
	typecfg.NewEnvSource("APP"), // APP_SERVER_PORT overrides server.port
)

cfg, err := loader.Load(ctx)
if err != nil {
	log.Fatal(err)
}
```

## Hot reload

File sources implement `Watchable`. Register callbacks before starting
the watcher:

```go
loader.OnReload(func(old, new *Config) {
	log.Printf("log level changed: %s -> %s", old.Log.Level, new.Log.Level)
})
loader.OnError(func(err error) {
	log.Printf("bad config edit ignored: %v", err) // previous config still active
})
loader.Watch(ctx)
defer loader.Stop()
```

If a reload produces an invalid config, the previous one keeps being
served — your service never ends up with no config at all.

## Custom sources

Implement `typecfg.Source` to plug in any backend:

```go
type Source interface {
	Name() string
	Load(ctx context.Context) (map[string]any, error)
}
```

For hot reload, also implement `typecfg.Watchable`. See
[`sources/yaml.go`](../sources/yaml.go) for a reference implementation.

## Validation rules

See [validation-rules.md](./validation-rules.md) for the full tag reference.

## Runnable example

A complete working example is in [`examples/basic/`](../examples/basic/).
