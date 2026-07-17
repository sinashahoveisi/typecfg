module github.com/sinashahoveisi/typecfg/integration

go 1.22

require (
	github.com/sinashahoveisi/typecfg v0.1.0
	github.com/sinashahoveisi/typecfg/jsonschema v0.1.0
	github.com/sinashahoveisi/typecfg/otel v0.1.0
	github.com/sinashahoveisi/typecfg/sources v0.1.0
	go.opentelemetry.io/otel/sdk/metric v1.32.0
)

require (
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.1 // indirect
	go.opentelemetry.io/otel v1.32.0 // indirect
	go.opentelemetry.io/otel/metric v1.32.0 // indirect
	go.opentelemetry.io/otel/sdk v1.32.0 // indirect
	go.opentelemetry.io/otel/trace v1.32.0 // indirect
	golang.org/x/sys v0.27.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/sinashahoveisi/typecfg => ../

replace github.com/sinashahoveisi/typecfg/sources => ../sources

replace github.com/sinashahoveisi/typecfg/jsonschema => ../jsonschema

replace github.com/sinashahoveisi/typecfg/otel => ../otel
