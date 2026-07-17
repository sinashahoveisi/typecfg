// Snippet from docs/codegen.md "Wire into Loader" (after typecfg-gen).
package main

import (
	"context"

	"github.com/sinashahoveisi/typecfg"
	"github.com/sinashahoveisi/typecfg/sources"
)

func main() {
	ctx := context.Background()
	loader := typecfg.NewGenerated[Config](
		ConfigBinder{},
		sources.NewYAMLFile("config.yaml"),
	)
	cfg, err := loader.Load(ctx)
	_ = cfg
	_ = err
}
