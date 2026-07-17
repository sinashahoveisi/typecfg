// Command typecfg-gen emits a GeneratedBinder implementation for a named
// config struct so Loader can bind/validate without reflecting over fields.
//
// Usage:
//
//	typecfg-gen -type Config -in config.go -out config_gen.go -package mypkg
//
// Typical go:generate directive in the file that defines the struct:
//
//	//go:generate go run github.com/sinashahoveisi/typecfg/cmd/typecfg-gen -type Config -in config.go -out config_gen.go -package mypkg
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	typeName := flag.String("type", "", "name of the config struct type to generate a binder for")
	inPath := flag.String("in", "", "input .go file containing the struct")
	outPath := flag.String("out", "", "output path for the generated binder file")
	pkgName := flag.String("package", "", "package name for the generated file")
	flag.Parse()

	if *typeName == "" || *inPath == "" || *outPath == "" || *pkgName == "" {
		fmt.Fprintln(os.Stderr, "usage: typecfg-gen -type Name -in file.go -out file_gen.go -package pkg")
		flag.PrintDefaults()
		os.Exit(2)
	}

	src, err := Generate(Options{
		TypeName:    *typeName,
		InputPath:   *inPath,
		PackageName: *pkgName,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "typecfg-gen: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*outPath, src, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "typecfg-gen: write %s: %v\n", *outPath, err)
		os.Exit(1)
	}
}
