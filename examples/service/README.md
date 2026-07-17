# Example: adopting typecfg for a small HTTP service

This is a fresh-eyes walkthrough of the documented public API — YAML
base config, env overrides, structured logging on reload, and hot
reload that keeps the last good config on failure.

## What it demonstrates

- A realistic `Config` (listen address/port, secret API key, log level
  with `oneof`, feature flags as `[]string`)
- `sources.NewYAMLFile` + `typecfg.NewEnvSource` merge order
- `SetLogger(slog.Default())` so reload success/failure shows up in logs
- `Watch` + `OnReload` / `OnError` (bad edits do not wipe the live config)

## Run

```bash
cd examples/service
go run .          # loads config.yaml; edit the file to trigger reload
go test -race -count=1 ./...
```

Env override example:

```bash
SVC_SERVER_PORT=9090 go run .
```
