# Code generation (typecfg-gen)

`typecfg-gen` emits a `GeneratedBinder[T]` implementation for a named config
struct so `NewGenerated` can bind and validate without reflecting over fields.
Hot reload, source merging, hooks, and error types are unchanged vs `New[T]`.

## Install / run

From the module root:

```bash
go run ./cmd/typecfg-gen -type Config -in config.go -out config_gen.go -package mypkg
```

Or install:

```bash
go install github.com/sinashahoveisi/typecfg/cmd/typecfg-gen@latest
typecfg-gen -type Config -in config.go -out config_gen.go -package mypkg
```

## go:generate

Place a directive next to the struct definition:

```go
package mypkg

//go:generate go run github.com/sinashahoveisi/typecfg/cmd/typecfg-gen -type Config -in config.go -out config_gen.go -package mypkg
type Config struct {
	Port int `cfg:"port" validate:"required,min=1"`
}
```

Then run `go generate ./...`.

## Wire into Loader

```go
loader := typecfg.NewGenerated[Config](
	ConfigBinder{},
	sources.NewYAMLFile("config.yaml"),
)
cfg, err := loader.Load(ctx)
```

`SetRawValidator`, `Watch`, `OnReload`, `OnError`, and `SetLogger` work the
same as with `New[Config]`.

## Unsupported fields

If a field type or tag combination is not supported, generation fails with an
error naming the field and reason (it does not skip the field). Unsupported
today: pointer fields, embedded fields, maps other than `map[string]string`,
and slice/element kinds outside the reflection binder's set.
