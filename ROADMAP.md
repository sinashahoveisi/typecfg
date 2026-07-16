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
- [x] Presence-based `required` validation (explicit zero no longer conflated with unset)
- [x] File watch survives atomic replace (rename/remove+create)
- [x] Core module has zero third-party dependencies (file sources isolated into sources/ submodule)

## Known gaps (post-v0.1)
- [ ] Parent-directory symlink swap (Kubernetes ConfigMap `..data` style atomic mount updates) is not yet handled — only the target file's own replacement is covered. Needs re-registration logic when the watched directory itself is swapped.
- [ ] `ConsulSource` / `EtcdSource` CI tests use fake clients only (injected KV/Watch fakes), not a real Consul or etcd server — wire-compatibility with live clusters is unverified as of this release.

## v0.2 — stronger validation and binding
- [x] Support `time.Time` (RFC3339 + custom `layout` tag), numeric slices ([]int/[]int8/../[]uint*/[]float32/[]float64) from both native YAML/JSON sequences and comma-separated strings, and `map[string]string` (native nested maps from YAML/JSON, JSON-encoded strings from flat sources like env)
- [x] More validate rules: `gt`, `lt`, `regexp`, `url`, `email`
- [x] Error messages with suggestions (e.g. "did you mean `Port`?")
- [x] Optional JSON Schema support for heavier validation

## v0.3 — more sources
- [x] `ConsulSource` (consul/ submodule; Consul KV + blocking Watch)
  — caveat: unit tests use a fake KV client; real Consul wire-compatibility is unverified in CI
- [x] `EtcdSource` (etcd/ submodule; etcd KV + revision Watch)
  — caveat: unit tests use a fake etcd client; real etcd wire-compatibility is unverified in CI
- [x] `RemoteHTTPSource` for an internal config server
- [x] Secret masking in logs/errors (fields tagged `secret:"true"` never printed in FieldError.Reason)

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
