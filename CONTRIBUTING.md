# Contributing

## Development setup

This repo uses a [Go workspace](https://go.dev/doc/tutorial/workspaces) to
link the core module, the `sources/` submodule, and examples locally:

```bash
git clone https://github.com/sinashahoveisi/typecfg.git
cd typecfg
# go.work is committed; no extra setup needed
```

## Running tests

From the repo root (core module):

```bash
go test -race ./...
```

From the sources submodule:

```bash
cd sources
go test -race ./...
```

Or run everything via the workspace:

```bash
go test -race ./...
go test -race ./sources/...
```

## Building the example

```bash
cd examples/basic
go build .
```

## Commit messages

Use the format `type: subject`:

- `feat:` new feature
- `fix:` bug fix
- `docs:` documentation only
- `test:` test additions or fixes
- `chore:` maintenance (deps, CI, tooling)

Example: `fix: distinguish required unset from explicit zero`

## Pull requests

- Keep the core module free of third-party dependencies
- File/JSON/YAML integrations belong in `sources/`
- Run `go test -race ./...` in both root and `sources/` before submitting
