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
- Struct binding uses reflection by default; optional zero-reflection codegen
  via [`typecfg-gen`](./docs/codegen.md) + `NewGenerated`.

Full runnable example in [`examples/basic/`](./examples/basic/).

## Status

v1.0.0 — the API described in this README and docs/ is frozen per
docs/versioning.md (breaking changes require a major version bump).
`FieldError.Reason` wording is explicitly excluded from that
guarantee — see docs/versioning.md.

Two things worth knowing before you rely on this in production:
- `consul.ConsulSource` and `etcd.EtcdSource` are tested against fake
  clients only; wire compatibility with a real Consul or etcd server
  has not been verified by this project. See docs/v1-readiness.md.
- v1.0 was verified through testing (including a fresh-eyes adoption
  example and full doc-snippet compilation), not an extended
  real-world production trial. See docs/v1-readiness.md for exactly
  what is and isn't covered.
