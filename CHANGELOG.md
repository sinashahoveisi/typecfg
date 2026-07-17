# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [1.0.0] - 2026-07-17

### Added

- `typecfg-gen` (`cmd/typecfg-gen`) emits a `GeneratedBinder[T]` for a named
  config struct; `NewGenerated` wires it into `Loader` with the same merge/
  Watch/hooks/`SetRawValidator` behavior as `New[T]` (see `docs/codegen.md`)
- `benchmarks/` submodule: reflection vs generated `Load` comparisons
  (`go test -bench=. -benchmem -count=3`)
- Generic `Loader[T]` with `Load`, `Get`, `Watch`, and `Stop`
- Sources: `EnvSource` (core), `YAMLFile`, `JSONFile`, and
  `RemoteHTTPSource` (`sources/` module; HTTP polling Watchable)
- `ConsulSource` in `consul/` submodule (Consul KV list + blocking-query Watch)
- `EtcdSource` in `etcd/` submodule (etcd Get/Watch with revision baseline)
- `Diff[T]` for config field changes; `Loader.SetLogger` for slog hot-reload logs (secret values redacted)
- `otel/` submodule: `Register` wires `config_reload_total` / `config_reload_errors_total`
- Ordered source merging with later sources overriding earlier ones
- Reflection-based binding with `cfg` and `default` tags
- Validation: `required`, `min`, `max`, `oneof` plus `Validator` interface
- File hot reload via fsnotify with fallback to previous config on error
- Precise errors: `FieldError`, `ValidationError`, `SourceError`
- Presence tracking for `required` (distinguishes unset from explicit zero)
- Directory-based file watching for atomic rename/replace updates
- Binding for `time.Time` (RFC3339 + optional `layout` tag), numeric
  slices (`[]int`, `[]int64`, `[]float64`, …), and `map[string]string`
  (nested map coercion or JSON from flat string sources)
- `secret:"true"` struct tag: bind/validate error messages redact the
  field value (`***REDACTED***`); oneof suggestions suppressed for secrets

### Changed

- **BREAKING:** `NewYAMLFile` / `YAMLFile` and `NewJSONFile` / `JSONFile`
  moved from `github.com/sinashahoveisi/typecfg` to
  `github.com/sinashahoveisi/typecfg/sources`. Update imports:

  ```go
  // before
  import "github.com/sinashahoveisi/typecfg"
  typecfg.NewYAMLFile("config.yaml")

  // after
  import (
      "github.com/sinashahoveisi/typecfg"
      "github.com/sinashahoveisi/typecfg/sources"
  )
  sources.NewYAMLFile("config.yaml")
  ```

  Core module now has zero third-party dependencies; file sources that
  pull in fsnotify/yaml.v3 live in the `sources/` submodule.
