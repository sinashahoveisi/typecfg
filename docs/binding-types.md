# Binding types

Besides the usual scalars (`string`, `bool`, ints, floats, `time.Duration`),
typecfg binds a few composite types with source-dependent input forms.

## time.Time

Default layout is RFC3339. Override with a `layout` struct tag (a
`time.Parse` layout string). No format sniffing or multi-format fallback.

```go
Start time.Time `cfg:"start"`                         // "2026-01-01T00:00:00Z"
Day   time.Time `cfg:"day" layout:"2006-01-02"`        // "2026-07-16"
```

`default:"..."` values are parsed with the same rule (RFC3339 unless
`layout` is set). Invalid values produce a `FieldError` that names the
expected layout.

## Numeric slices

Supported element kinds: signed ints, unsigned ints, `float32`, and
`float64` (plus `[]string`, which already existed).

Two input forms:

1. **Native sequence** (YAML list / JSON array) — elements converted
   individually without stringifying the whole slice first.
2. **Comma-separated string** (typical for env vars) — split on `,`,
   trim whitespace, then parse each element.

```yaml
# native YAML
ports: [8080, 8443]
# or
ports:
  - 8080
  - 8443
```

```bash
# env / flat string
APP_PORTS=8080,8443
```

A bad element fails the whole field and names the index and value, e.g.
`element 1 ("bad") is not a valid int`.

## map[string]string

Two input forms:

1. **Native nested map** from YAML/JSON — each value coerced with `%v`.
2. **JSON object string** from flat sources (e.g. env) — unmarshaled with
   `encoding/json` into `map[string]string`.

```yaml
labels:
  env: prod
  team: backend
```

```bash
APP_LABELS={"env":"prod","team":"backend"}
```

`default` must be a JSON object string when used on a map field. There is
no `key=val,key=val` format.
