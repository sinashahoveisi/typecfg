package typecfg

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestBind_TimeRFC3339(t *testing.T) {
	type cfg struct {
		Start time.Time `cfg:"start"`
	}
	loader := New[cfg](mapSource{
		name: "test",
		data: map[string]any{"start": "2026-01-01T00:00:00Z"},
	})
	got, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !got.Start.Equal(want) {
		t.Errorf("Start = %v, want %v", got.Start, want)
	}
}

func TestBind_TimeCustomLayout(t *testing.T) {
	type cfg struct {
		Start time.Time `cfg:"start" layout:"2006-01-02"`
	}
	loader := New[cfg](mapSource{
		name: "test",
		data: map[string]any{"start": "2026-07-16"},
	})
	got, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	if !got.Start.Equal(want) {
		t.Errorf("Start = %v, want %v", got.Start, want)
	}
}

func TestBind_TimeInvalidFormat(t *testing.T) {
	type cfg struct {
		Start time.Time `cfg:"start"`
	}
	loader := New[cfg](mapSource{
		name: "test",
		data: map[string]any{"start": "01/01/2026"},
	})
	_, err := loader.Load(context.Background())
	if err == nil {
		t.Fatal("expected parse error")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	reason := verr.Errors[0].Reason
	t.Logf("reason: %s", reason)
	if !strings.Contains(reason, "RFC3339") {
		t.Errorf("Reason should name RFC3339, got %q", reason)
	}
	if !strings.Contains(reason, "01/01/2026") {
		t.Errorf("Reason should quote the bad value, got %q", reason)
	}
}

func TestBind_TimeInvalidCustomLayout(t *testing.T) {
	type cfg struct {
		Start time.Time `cfg:"start" layout:"2006-01-02"`
	}
	loader := New[cfg](mapSource{
		name: "test",
		data: map[string]any{"start": "2026-01-01T00:00:00Z"},
	})
	_, err := loader.Load(context.Background())
	if err == nil {
		t.Fatal("expected parse error")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	reason := verr.Errors[0].Reason
	if !strings.Contains(reason, "2006-01-02") {
		t.Errorf("Reason should name custom layout, got %q", reason)
	}
}

func TestBind_TimeDefault(t *testing.T) {
	type cfg struct {
		Start time.Time `cfg:"start" default:"2026-01-01T00:00:00Z"`
	}
	loader := New[cfg](mapSource{name: "test", data: map[string]any{}})
	got, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !got.Start.Equal(want) {
		t.Errorf("Start = %v, want %v (from default)", got.Start, want)
	}
}

func TestBind_IntSliceValid(t *testing.T) {
	type cfg struct {
		Ports []int `cfg:"ports"`
	}
	loader := New[cfg](mapSource{
		name: "test",
		data: map[string]any{"ports": "8080, 9090, 3000"},
	})
	got, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []int{8080, 9090, 3000}
	if len(got.Ports) != len(want) {
		t.Fatalf("Ports = %v, want %v", got.Ports, want)
	}
	for i := range want {
		if got.Ports[i] != want[i] {
			t.Errorf("Ports[%d] = %d, want %d", i, got.Ports[i], want[i])
		}
	}
}

func TestBind_IntSliceInvalidElement(t *testing.T) {
	type cfg struct {
		Ports []int `cfg:"ports"`
	}
	loader := New[cfg](mapSource{
		name: "test",
		data: map[string]any{"ports": "1,two,3"},
	})
	_, err := loader.Load(context.Background())
	if err == nil {
		t.Fatal("expected parse error")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	reason := verr.Errors[0].Reason
	if !strings.Contains(reason, `element 1 ("two")`) {
		t.Errorf("Reason should name index and bad value, got %q", reason)
	}
}

func TestBind_IntSliceEmpty(t *testing.T) {
	type cfg struct {
		Ports []int `cfg:"ports"`
	}
	loader := New[cfg](mapSource{
		name: "test",
		data: map[string]any{"ports": ""},
	})
	got, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Ports == nil || len(got.Ports) != 0 {
		t.Errorf("Ports = %v, want empty non-nil slice", got.Ports)
	}
}

func TestBind_IntSliceDefault(t *testing.T) {
	type cfg struct {
		Ports []int `cfg:"ports" default:"80,443"`
	}
	loader := New[cfg](mapSource{name: "test", data: map[string]any{}})
	got, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Ports) != 2 || got.Ports[0] != 80 || got.Ports[1] != 443 {
		t.Errorf("Ports = %v, want [80 443]", got.Ports)
	}
}

func TestBind_IntSliceFromEnvCommaString(t *testing.T) {
	type cfg struct {
		Ports []int `cfg:"ports"`
	}
	t.Setenv("APP_PORTS", "8080,8443")
	loader := New[cfg](NewEnvSource("APP"))
	got, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Ports) != 2 || got.Ports[0] != 8080 || got.Ports[1] != 8443 {
		t.Errorf("Ports = %v, want [8080 8443]", got.Ports)
	}
}

func TestBind_StringSliceFromEnvCommaString(t *testing.T) {
	type cfg struct {
		Names []string `cfg:"names"`
	}
	t.Setenv("APP_NAMES", "api, worker")
	loader := New[cfg](NewEnvSource("APP"))
	got, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Names) != 2 || got.Names[0] != "api" || got.Names[1] != "worker" {
		t.Errorf("Names = %v, want [api worker]", got.Names)
	}
}

func TestBind_Float64SliceValid(t *testing.T) {
	type cfg struct {
		Rates []float64 `cfg:"rates"`
	}
	loader := New[cfg](mapSource{
		name: "test",
		data: map[string]any{"rates": "1.5, 2.0, 0.25"},
	})
	got, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []float64{1.5, 2.0, 0.25}
	if len(got.Rates) != len(want) {
		t.Fatalf("Rates = %v, want %v", got.Rates, want)
	}
	for i := range want {
		if got.Rates[i] != want[i] {
			t.Errorf("Rates[%d] = %v, want %v", i, got.Rates[i], want[i])
		}
	}
}

func TestBind_Float64SliceInvalidElement(t *testing.T) {
	type cfg struct {
		Rates []float64 `cfg:"rates"`
	}
	loader := New[cfg](mapSource{
		name: "test",
		data: map[string]any{"rates": "1.0,nope,3.0"},
	})
	_, err := loader.Load(context.Background())
	if err == nil {
		t.Fatal("expected parse error")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	reason := verr.Errors[0].Reason
	if !strings.Contains(reason, `element 1 ("nope")`) {
		t.Errorf("Reason should name index and bad value, got %q", reason)
	}
}

func TestBind_StringMapFromNested(t *testing.T) {
	type cfg struct {
		Labels map[string]string `cfg:"labels"`
	}
	loader := New[cfg](mapSource{
		name: "test",
		data: map[string]any{
			"labels": map[string]any{"env": "prod", "team": "backend"},
		},
	})
	got, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Labels["env"] != "prod" || got.Labels["team"] != "backend" {
		t.Errorf("Labels = %v, want env=prod team=backend", got.Labels)
	}
}

func TestBind_StringMapFromEnvJSON(t *testing.T) {
	type cfg struct {
		Labels map[string]string `cfg:"labels"`
	}
	t.Setenv("APP_LABELS", `{"env":"prod","team":"backend"}`)
	loader := New[cfg](NewEnvSource("APP"))
	got, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Labels["env"] != "prod" || got.Labels["team"] != "backend" {
		t.Errorf("Labels = %v, want env=prod team=backend", got.Labels)
	}
}

func TestBind_StringMapFromEnvInvalidJSON(t *testing.T) {
	type cfg struct {
		Labels map[string]string `cfg:"labels"`
	}
	t.Setenv("APP_LABELS", `not-json`)
	loader := New[cfg](NewEnvSource("APP"))
	_, err := loader.Load(context.Background())
	if err == nil {
		t.Fatal("expected JSON parse error")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	reason := verr.Errors[0].Reason
	t.Logf("reason: %s", reason)
	if !strings.Contains(reason, `"not-json"`) {
		t.Errorf("Reason should quote raw string, got %q", reason)
	}
	if !strings.Contains(reason, "invalid") && !strings.Contains(reason, "JSON") {
		t.Errorf("Reason should mention JSON error, got %q", reason)
	}
}

func TestBind_StringMapDefault(t *testing.T) {
	type cfg struct {
		Labels map[string]string `cfg:"labels" default:"{\"env\":\"dev\"}"`
	}
	loader := New[cfg](mapSource{name: "test", data: map[string]any{}})
	got, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Labels["env"] != "dev" {
		t.Errorf("Labels = %v, want env=dev", got.Labels)
	}
}
