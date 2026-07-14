# Validation rules

typecfg validates config structs using the `validate` struct tag. Rules are
comma-separated. All violations are collected in a single pass.

## Tag reference

| Rule | Tag example | Applies to | Meaning |
|---|---|---|---|
| `required` | `validate:"required"` | any | Field must be present in source data or satisfied by a `default` tag. Explicit zero values (e.g. `port: 0`) count as present. |
| `min` | `validate:"min=1"` | numbers, strings | Numeric fields must be >= N. For strings, checks length >= N. |
| `max` | `validate:"max=65535"` | numbers, strings | Numeric fields must be <= N. For strings, checks length <= N. |
| `oneof` | `validate:"oneof=debug info warn error"` | any | Value must match one of the space-separated options. |

For cross-field validation (e.g. `StartPort < EndPort`), implement
`Validate() error` on the struct — it is called automatically after tag
rules pass.

## Examples

### required

```go
Port int `cfg:"port" validate:"required"`
```

- YAML with `port: 0` → passes (explicitly set)
- YAML with no `port` key → fails: `is required but was not set`
- No key but `default:"8080"` → passes (default counts as present)

### min / max

```go
Port int `cfg:"port" validate:"required,min=1,max=65535"`
```

- `port: 0` → fails `min=1` (present but out of range)
- `port: 70000` → fails `max=65535`

```go
Name string `cfg:"name" validate:"min=3,max=32"`
```

- `name: "ab"` → fails `min=3` (length check)

### oneof

```go
Level string `cfg:"level" default:"info" validate:"oneof=debug info warn error"`
```

- `level: verbose` → fails: `must be one of [debug info warn error], got "verbose"`
- `level: debug` → passes
