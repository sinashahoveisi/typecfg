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
