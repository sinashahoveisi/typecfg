package typecfg

import "time"

// ParityConfig exercises every bind/validate surface the generator supports.
// Used by gen_parity_test.go; binder is generated into parity_config_gen.go.
//
//go:generate go run ./cmd/typecfg-gen -type ParityConfig -in parity_config.go -out parity_config_gen.go -package typecfg
type ParityConfig struct {
	Name    string        `cfg:"name" default:"anon"`
	Enabled bool          `cfg:"enabled" default:"true"`
	Count   int           `cfg:"count" validate:"required,min=0,max=1000,gt=-1,lt=1001"`
	Ratio   float64       `cfg:"ratio"`
	Timeout time.Duration `cfg:"timeout" default:"1s"`
	Start   time.Time     `cfg:"start"`
	Day     time.Time     `cfg:"day" layout:"2006-01-02"`

	Tags   []string       `cfg:"tags"`
	Ports  []int          `cfg:"ports"`
	Rates  []float64      `cfg:"rates"`
	Labels map[string]string `cfg:"labels"`

	Mode  string `cfg:"mode" validate:"oneof=dev prod"`
	Code  string `cfg:"code" validate:"regexp=^[A-Z]+$"`
	Home  string `cfg:"home" validate:"url"`
	Email string `cfg:"email" validate:"email"`

	Token string `cfg:"token" secret:"true" validate:"oneof=a b c"`

	Server struct {
		Host string `cfg:"host" default:"0.0.0.0"`
		Port int    `cfg:"port" validate:"required,min=1,max=65535"`
	} `cfg:"server"`
}
