module github.com/sinashahoveisi/typecfg/benchmarks

go 1.22

require github.com/sinashahoveisi/typecfg v0.1.0

// Local development: pin to the repo root. Tagged releases can drop this
// once a published typecfg version includes NewGenerated / ParityConfigBinder.
replace github.com/sinashahoveisi/typecfg => ../
