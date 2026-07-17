module github.com/sinashahoveisi/typecfg/examples/doc-snippets

go 1.22

require (
	github.com/sinashahoveisi/typecfg v0.1.0
	github.com/sinashahoveisi/typecfg/jsonschema v0.1.0
	github.com/sinashahoveisi/typecfg/sources v0.1.0
)

require (
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.1 // indirect
	golang.org/x/sys v0.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/sinashahoveisi/typecfg => ../..

replace github.com/sinashahoveisi/typecfg/sources => ../../sources

replace github.com/sinashahoveisi/typecfg/jsonschema => ../../jsonschema
