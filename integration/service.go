package integration

import "time"

// ServiceConfig is a realistic service-shaped config for end-to-end tests:
// nested server block, slices, maps, time.Time, and a secret API key.
//
//go:generate go run ../cmd/typecfg-gen -type ServiceConfig -in service.go -out service_gen.go -package integration
type ServiceConfig struct {
	Server struct {
		Host string `cfg:"host" default:"0.0.0.0"`
		Port int    `cfg:"port" validate:"required,min=1,max=65535"`
	} `cfg:"server"`

	Features  []string          `cfg:"features"`
	Labels    map[string]string `cfg:"labels"`
	StartedAt time.Time         `cfg:"started_at"`
	APIKey    string            `cfg:"api_key" secret:"true" validate:"required"`
	LogLevel  string            `cfg:"log_level" default:"info" validate:"oneof=debug info warn error"`
}
