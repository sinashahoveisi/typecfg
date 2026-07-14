package typecfg

import (
	"context"
	"errors"
	"testing"
)

type testConfig struct {
	Server struct {
		Host string `cfg:"host" default:"0.0.0.0"`
		Port int    `cfg:"port" validate:"required,min=1,max=65535"`
	} `cfg:"server"`
	Log struct {
		Level string `cfg:"level" default:"info" validate:"oneof=debug info warn error"`
	} `cfg:"log"`
}

type requiredPortConfig struct {
	Port int `cfg:"port" validate:"required"`
}

type defaultRequiredConfig struct {
	Level string `cfg:"level" default:"info" validate:"required"`
}

type mapSource struct {
	name string
	data map[string]any
}

func (m mapSource) Name() string { return m.name }

func (m mapSource) Load(_ context.Context) (map[string]any, error) {
	return m.data, nil
}

func TestLoad_RequiredMissing(t *testing.T) {
	loader := New[testConfig](mapSource{
		name: "test",
		data: map[string]any{
			"log": map[string]any{"level": "info"},
		},
	})

	_, err := loader.Load(context.Background())
	if err == nil {
		t.Fatal("expected error for missing required field")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	var found bool
	for _, e := range verr.Errors {
		if e.Field == "server.port" && e.Tag == "required" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected required error for server.port, got: %+v", verr.Errors)
	}
}

func TestLoad_RequiredExplicitZero(t *testing.T) {
	loader := New[requiredPortConfig](mapSource{
		name: "test",
		data: map[string]any{"port": 0},
	})

	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("port: 0 explicitly set should not fail required, got: %v", err)
	}
	if cfg.Port != 0 {
		t.Errorf("Port = %d, want 0", cfg.Port)
	}
}

func TestLoad_RequiredAbsent(t *testing.T) {
	loader := New[requiredPortConfig](mapSource{
		name: "test",
		data: map[string]any{"other": "value"},
	})

	_, err := loader.Load(context.Background())
	if err == nil {
		t.Fatal("expected error when required port key is absent")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	if len(verr.Errors) != 1 || verr.Errors[0].Field != "port" || verr.Errors[0].Tag != "required" {
		t.Fatalf("unexpected errors: %+v", verr.Errors)
	}
}

func TestLoad_DefaultCountsAsPresent(t *testing.T) {
	loader := New[defaultRequiredConfig](mapSource{
		name: "test",
		data: map[string]any{"other": "value"},
	})

	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("default value should satisfy required, got: %v", err)
	}
	if cfg.Level != "info" {
		t.Errorf("Level = %q, want info (from default)", cfg.Level)
	}
}

func TestLoad_OneOfViolation(t *testing.T) {
	loader := New[testConfig](mapSource{
		name: "test",
		data: map[string]any{
			"server": map[string]any{"port": 8080},
			"log":    map[string]any{"level": "verbose"},
		},
	})

	_, err := loader.Load(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid oneof value")
	}
}

func TestEnvOverridesMapSource(t *testing.T) {
	t.Setenv("APP_SERVER_PORT", "9090")

	loader := New[testConfig](
		mapSource{
			name: "test",
			data: map[string]any{
				"server": map[string]any{"port": 8080},
				"log":    map[string]any{"level": "info"},
			},
		},
		NewEnvSource("APP"),
	)
	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Port = %d, want 9090 (env should override map source)", cfg.Server.Port)
	}
}
