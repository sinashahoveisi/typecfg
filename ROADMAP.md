# Roadmap

## v0.1 (this release — initial skeleton)
- [x] Generic `Loader[T]` with `Load` / `Get`
- [x] Sources: `YAMLFile`, `JSONFile`, `EnvSource`
- [x] Merge multiple sources with ordered override
- [x] Reflection-based binding with `cfg`, `default` tags
- [x] Validation: `required`, `min`, `max`, `oneof` + `Validator` interface for cross-field checks
- [x] File hot reload via `fsnotify`, falling back to the previous config on error
- [x] Precise errors with field name + source (`FieldError`, `ValidationError`, `SourceError`)
- [x] Basic test suite

## v0.2 — stronger validation and binding
- [ ] Support `time.Time`, `[]int`, `map[string]string` in bind
- [ ] More validate rules: `gt`, `lt`, `regexp`, `url`, `email`
- [ ] Error messages with suggestions (e.g. "did you mean `Port`?")
- [ ] Optional JSON Schema support for heavier validation

## v0.3 — more sources
- [ ] `ConsulSource` / `EtcdSource` (implementing `Watchable`)
- [ ] `RemoteHTTPSource` for an internal config server
- [ ] Secret masking in logs/errors (fields tagged `secret:"true"` never printed)

## v0.4 — observability integration
- Inspired by the previous package (Circuit Breaker + Retry + Observability):
- [ ] `OnReload`/`OnError` hooks wired to OpenTelemetry metrics out of the box (`config_reload_total`, `config_reload_errors_total`)
- [ ] Optional structured logging (slog) per reload, including a diff of changed fields

## v0.5 — codegen (optional, zero-reflection)
- [ ] `typecfg generate` command that emits Bind/Validate code at build time (similar to `cfgx`, but with hot-reload support, not just build-time baking)
- [ ] Reflection vs codegen benchmarks

## v1.0
- [ ] API freeze
- [ ] Test coverage above 80%
- [ ] Full docs + production examples (k8s ConfigMap hot reload, multi-env)
- [ ] Formal semantic versioning commitment
