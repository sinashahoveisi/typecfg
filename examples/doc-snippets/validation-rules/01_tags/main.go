// Snippets from docs/validation-rules.md (required / min-max / oneof).
package main

type Config struct {
	Port  int    `cfg:"port" validate:"required"`
	Port2 int    `cfg:"port2" validate:"required,min=1,max=65535"`
	Name  string `cfg:"name" validate:"min=3,max=32"`
	Level string `cfg:"level" default:"info" validate:"oneof=debug info warn error"`
}

func main() {
	_ = Config{}
}
