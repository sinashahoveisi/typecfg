# Versioning Policy

typecfg follows [Semantic Versioning](https://semver.org) starting at v1.0.0.

## PATCH (v1.0.x)
Bug fixes that restore documented behavior without changing it.
Example from this project's history: the `required` validation fix
that stopped conflating an explicit zero value with an absent field
(pre-v0.2). Had this shipped after v1.0, it would have been a patch —
documented behavior didn't change, a bug that contradicted it was fixed.

## MINOR (v1.x.0)
Additive changes that don't break existing code.
Examples: `SetLogger`, `Diff`, `SetRawValidator`, `NewGenerated` — all
new exported identifiers that didn't touch any existing signature.
New validate tag rules (e.g. `gt`, `lt`) are minor for the same reason.
New Source implementations (e.g. `ConsulSource`) are minor — a new
submodule touches no existing API.

## MAJOR (v2.0.0)
Anything that breaks existing user code.
Example from this project's pre-v1.0 history (which would have
required a major bump had it happened post-freeze): relocating
`NewYAMLFile`/`NewJSONFile` from the root package into `sources/` —
the import path changed and existing code stopped compiling.

## Project-specific rules
- Changing the exact wording of a FieldError.Reason message is treated
  as MINOR, not PATCH — message text isn't contractually stable, but
  callers who assert on substrings can still break, so this must be
  called out explicitly in CHANGELOG.md even though it isn't a major
  bump.
- Changing a documented default (e.g. RemoteHTTPSource's default
  PollInterval) is MINOR if the field remains configurable, but must
  be labeled "behavior change" in CHANGELOG.md regardless of version
  bump size.

## Versioning nested modules
`sources/`, `jsonschema/`, `consul/`, `etcd/`, and `otel/` are each
independent Go modules with their own go.mod and must be tagged
separately (e.g. `sources/vX.Y.Z`, not just the root `vX.Y.Z`). Their
version numbers are not required to stay in sync with the root
module's — a submodule with no changes can lag behind several root
minor releases.

Before go.work's `replace` directives can be safely removed in favor
of plain `use` entries, every module actually depended upon must have
at least one published tag — this project hit exactly this problem
early on when `sources/` had no `sources/v0.1.0` tag yet, forcing
versioned `replace` directives to remain in go.work.
