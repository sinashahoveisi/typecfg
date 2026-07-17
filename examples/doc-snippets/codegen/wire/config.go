package main

//go:generate go run ../../../../cmd/typecfg-gen -type Config -in config.go -out config_gen.go -package main

type Config struct {
	Port int `cfg:"port" validate:"required,min=1"`
}
