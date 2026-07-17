# v1.0 Readiness Assessment

This document records which parts of typecfg's exported API are
considered stable going into v1.0 versus which carry known residual
risk, so post-v1.0 changes to the riskier surfaces can be made
deliberately rather than accidentally breaking an implied promise.

## Stable — frozen at v1.0

- `Loader[T]`: `New`, `Load`, `Get`, `Watch`, `Stop`, `OnReload`,
  `OnError`, `SetLogger`, `SetRawValidator`
- `Source`, `Watchable` interfaces
- `FieldError`, `ValidationError`, `SourceError`, `SchemaError`
- Struct tags: `cfg`, `default`, `validate` (all current rules:
  required, min, max, gt, lt, oneof, regexp, url, email), `secret`,
  `layout`
- `sources.YAMLFile`, `sources.JSONFile`, `sources.RemoteHTTPSource`
- Root `EnvSource`
- `Diff`, `FieldChange`

## Additive but young — usable, watched closely

- `NewGenerated` / `GeneratedBinder` (v0.5) — behaviorally proven
  identical to reflection via parity + integration tests, but not yet
  exercised by any real project outside this repo.
- `consul.ConsulSource`, `etcd.EtcdSource` — internal logic
  (key-nesting, error wrapping, watch-loop shape, the Load-to-Watch
  gap fix) is well-tested, but ALL existing tests use fake clients.
  Wire-level compatibility with a real Consul or etcd server has never
  been verified in this project. This is the largest known-unknown
  going into v1.0.
- `otel.Register`, `jsonschema.Validator` — thin wrappers around
  external libraries (go.opentelemetry.io/otel,
  santhosh-tekuri/jsonschema). Our wrapper code is stable, but a
  breaking change in either upstream dependency could force a typecfg
  major bump independent of anything we control.

## Explicitly not a stability contract

`FieldError.Reason`'s exact wording is not covered by semver. Per
docs/versioning.md, wording changes are treated as MINOR (not PATCH),
but are not guaranteed to stay byte-identical across any version
boundary. Code that asserts on Reason substrings should expect to
revisit those assertions on upgrades.

## Recommended before relying on Consul/Etcd in production

Run a manual smoke test against a real Consul and/or etcd instance
before depending on those sources for anything critical. This project
has not done so as of v1.0.

## Real-world usage period — substituted with test coverage

The project checklist originally called for a real-world usage period
before v1.0. That was substituted with test-based verification instead
(examples/doc-snippets/ compiling every documented code sample,
examples/service/ as a fresh-eyes adoption example under -race).

This substitute covers: stale or broken public-API documentation,
compile-time friction, and a first-adoption path (YAML+env layering,
slog integration, Watch-based hot reload, keep-last-good-config
behavior) exercised under the race detector.

This substitute does NOT cover: multi-day production load, Kubernetes
ConfigMap `..data` symlink-swap mounts (tracked separately under Known
gaps), real Consul/etcd wire compatibility (tests use fake clients —
see the Consul/Etcd entry above), operational failure modes like
partial writes under load or permission changes, secret-manager
integration patterns, or whether the API remains ergonomic after weeks
of real, evolving usage. Treat v1.0 as verified-by-testing, not
verified-by-production-experience.
