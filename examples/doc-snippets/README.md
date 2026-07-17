# Doc snippet compilation checks

Extracted Go snippets from `README.md` and `docs/` that a user would
write and run. Layout: **one subdirectory per doc page**, then one
package per snippet (`01_…`, `02_…`, …).

Build every snippet:

```bash
cd examples/doc-snippets
for d in readme/*/ guide/*/ binding-types/*/ validation-rules/*/ json-schema/*/ codegen/wire; do
  echo "==> $d"
  go build -o /dev/null "./$d"
done
```

## Excluded (not user-runnable Go)

| Location | Why |
|---|---|
| `docs/guide.md` — `Source` interface block | Documents the library interface contract; not application code |
| `docs/binding-types.md` — YAML / bash blocks | Not Go |
| `docs/versioning.md` | No Go snippets |
| `docs/v1-readiness.md` | No Go snippets |
| Install / `go get` / `go run` bash blocks | Shell, not Go |
| `docs/codegen.md` — `go:generate` package block | Compiles as a library after `typecfg-gen`; covered via `codegen/wire` which embeds the generated binder the directive would produce |

Fragments (struct fields, partial loaders) are wrapped with a real
`main`, temp config files, and imports so `go build` succeeds. That
surrounding context is scaffolding only — the core lines match the docs.
