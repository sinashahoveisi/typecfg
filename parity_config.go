package typecfg

import "time"

// ParityConfig is a representative config used by gen_parity_test.go and the
// benchmarks submodule to compare reflection vs generated bind/validate.
//
//go:generate go run ./cmd/typecfg-gen -type ParityConfig -in parity_config.go -out parity_config_gen.go -package typecfg
type ParityConfig struct {
	// Name is a display name; defaults to "anon" when unset.
	Name string `cfg:"name" default:"anon"`
	// Enabled toggles the feature; defaults to true when unset.
	Enabled bool `cfg:"enabled" default:"true"`
	// Count is required and range-checked by validate tags.
	Count int `cfg:"count" validate:"required,min=0,max=1000,gt=-1,lt=1001"`
	// Ratio is an optional float64.
	Ratio float64 `cfg:"ratio"`
	// Timeout defaults to 1s when unset.
	Timeout time.Duration `cfg:"timeout" default:"1s"`
	// Start is RFC3339 time.Time.
	Start time.Time `cfg:"start"`
	// Day uses a custom layout tag (date only).
	Day time.Time `cfg:"day" layout:"2006-01-02"`

	// Tags is a string slice (comma-string or native list).
	Tags []string `cfg:"tags"`
	// Ports is an int slice (comma-string or native list).
	Ports []int `cfg:"ports"`
	// Rates is a float64 slice.
	Rates []float64 `cfg:"rates"`
	// Labels is map[string]string (nested map or JSON string).
	Labels map[string]string `cfg:"labels"`

	// Mode must be one of "dev" or "prod".
	Mode string `cfg:"mode" validate:"oneof=dev prod"`
	// Code must match ^[A-Z]+$.
	Code string `cfg:"code" validate:"regexp=^[A-Z]+$"`
	// Home must be a valid URL.
	Home string `cfg:"home" validate:"url"`
	// Email must be a valid email address.
	Email string `cfg:"email" validate:"email"`

	// Token is secret-redacted in FieldError reasons; oneof a|b|c.
	Token string `cfg:"token" secret:"true" validate:"oneof=a b c"`

	// Server nests host/port under cfg:"server".
	Server struct {
		// Host defaults to 0.0.0.0.
		Host string `cfg:"host" default:"0.0.0.0"`
		// Port is required in 1..65535.
		Port int `cfg:"port" validate:"required,min=1,max=65535"`
	} `cfg:"server"`
}
