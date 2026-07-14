# typecfg

A small, type-safe config loader for Go, focused on three things most
projects either don't have at all or drag in `viper` for:

1. **Type safety via generics** — `typecfg.New[Config](...)` gives you back
   a typed `*Config`, not a `map[string]any` you have to cast.
2. **Real hot reload** — file sources are watched with `fsnotify`; if an
   edit produces an invalid config, the previous one keeps being served
   (your service never ends up with no config at all).
3. **Clear errors** — instead of `field X is required`, you get
   `field "server.port" (sources: [cfg:port env:APP_SERVER_PORT]) is
   required but was not set`, and every problem is collected in one pass
   instead of failing on the first one.

The core module has **zero third-party dependencies**. Optional file
sources (YAML/JSON with hot reload) live in
[`sources/`](./sources/).

## Install

```bash
go get github.com/sinashahoveisi/typecfg
go get github.com/sinashahoveisi/typecfg/sources   # YAML/JSON file sources
```

## Quick start

```go
import (
	"github.com/sinashahoveisi/typecfg"
	"github.com/sinashahoveisi/typecfg/sources"
)

loader := typecfg.New[Config](
	sources.NewYAMLFile("config.yaml"),
	typecfg.NewEnvSource("APP"),
)
cfg, err := loader.Load(ctx)
```

See the [getting-started guide](./docs/guide.md) for the full walkthrough,
including hot reload, custom sources, and validation.

## Design

- **`Source`** — small interface (`Load(ctx) (map[string]any, error)`).
  `EnvSource` is in the core; `YAMLFile`/`JSONFile` are in `sources/`.
- Sources merge in order; the last one wins.
- **`Watchable`** — optional interface for hot reload (file sources in `sources/`).
- Struct binding uses reflection; a zero-reflection codegen mode is on the
  [roadmap](./ROADMAP.md).

Full runnable example in [`examples/basic/`](./examples/basic/).

## Status

v0.1 — API may still change. See [CHANGELOG.md](./CHANGELOG.md).
