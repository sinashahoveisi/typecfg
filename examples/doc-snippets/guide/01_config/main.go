// Snippet from docs/guide.md "Define your config struct".
package main

type Config struct {
	Server struct {
		Host string `cfg:"host" default:"0.0.0.0"`
		Port int    `cfg:"port" validate:"required,min=1,max=65535"`
	} `cfg:"server"`
	Log struct {
		Level string `cfg:"level" default:"info" validate:"oneof=debug info warn error"`
	} `cfg:"log"`
}

func main() {
	_ = Config{}
}
