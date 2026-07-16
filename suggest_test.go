package typecfg

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSuggest_RequiredCloseSibling(t *testing.T) {
	type cfg struct {
		Port int `cfg:"port" validate:"required"`
	}
	_, err := New[cfg](mapSource{
		name: "t",
		data: map[string]any{"prot": 8080},
	}).Load(context.Background())
	if err == nil {
		t.Fatal("expected required error")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("got %T: %v", err, err)
	}
	reason := verr.Errors[0].Reason
	t.Logf("reason: %s", reason)
	want := `is required but was not set (did you mean "prot"? a similar key was found in the source)`
	if reason != want {
		t.Errorf("Reason = %q, want %q", reason, want)
	}
}

func TestSuggest_RequiredNoCloseSibling(t *testing.T) {
	type cfg struct {
		Port int `cfg:"port" validate:"required"`
	}
	_, err := New[cfg](mapSource{
		name: "t",
		data: map[string]any{"completely_unrelated": "x"},
	}).Load(context.Background())
	if err == nil {
		t.Fatal("expected required error")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("got %T: %v", err, err)
	}
	reason := verr.Errors[0].Reason
	t.Logf("reason: %s", reason)
	if reason != "is required but was not set" {
		t.Errorf("Reason = %q, want unchanged required message", reason)
	}
	if strings.Contains(reason, "did you mean") {
		t.Errorf("should not suggest, got %q", reason)
	}
}

func TestSuggest_RequiredAmbiguousSiblings(t *testing.T) {
	type cfg struct {
		Port int `cfg:"port" validate:"required"`
	}
	// "prot" and "prat" are both distance 2 from "port".
	_, err := New[cfg](mapSource{
		name: "t",
		data: map[string]any{"prot": 1, "prat": 2},
	}).Load(context.Background())
	if err == nil {
		t.Fatal("expected required error")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("got %T: %v", err, err)
	}
	reason := verr.Errors[0].Reason
	t.Logf("reason: %s", reason)
	if reason != "is required but was not set" {
		t.Errorf("Reason = %q, want no suggestion when ambiguous", reason)
	}
}

func TestSuggest_OneOfCloseTypo(t *testing.T) {
	type cfg struct {
		Level string `cfg:"level" validate:"oneof=debug info warn error"`
	}
	_, err := New[cfg](mapSource{
		name: "t",
		data: map[string]any{"level": "ifno"},
	}).Load(context.Background())
	if err == nil {
		t.Fatal("expected oneof error")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("got %T: %v", err, err)
	}
	reason := verr.Errors[0].Reason
	t.Logf("reason: %s", reason)
	want := `must be one of [debug info warn error], got "ifno" (did you mean "info"?)`
	if reason != want {
		t.Errorf("Reason = %q, want %q", reason, want)
	}
}

func TestSuggest_OneOfFarValue(t *testing.T) {
	type cfg struct {
		Level string `cfg:"level" validate:"oneof=debug info warn error"`
	}
	_, err := New[cfg](mapSource{
		name: "t",
		data: map[string]any{"level": "verbose"},
	}).Load(context.Background())
	if err == nil {
		t.Fatal("expected oneof error")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("got %T: %v", err, err)
	}
	reason := verr.Errors[0].Reason
	t.Logf("reason: %s", reason)
	want := `must be one of [debug info warn error], got "verbose"`
	if reason != want {
		t.Errorf("Reason = %q, want %q", reason, want)
	}
	if strings.Contains(reason, "did you mean") {
		t.Errorf("should not suggest, got %q", reason)
	}
}

func TestSuggest_RequiredNoCrossNesting(t *testing.T) {
	// Top-level key "prot" is Levenshtein-close to nested field key "port",
	// but must NOT be suggested for Server.Port — different map level.
	type cfg struct {
		Server struct {
			Port int `cfg:"port" validate:"required"`
		} `cfg:"server"`
	}
	_, err := New[cfg](mapSource{
		name: "t",
		data: map[string]any{
			"prot":   9999,
			"server": map[string]any{},
		},
	}).Load(context.Background())
	if err == nil {
		t.Fatal("expected required error for server.port")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("got %T: %v", err, err)
	}
	var reason string
	for _, e := range verr.Errors {
		if e.Field == "server.port" && e.Tag == "required" {
			reason = e.Reason
			break
		}
	}
	if reason == "" {
		t.Fatalf("expected required error on server.port, got %+v", verr.Errors)
	}
	t.Logf("reason: %s", reason)
	if reason != "is required but was not set" {
		t.Errorf("Reason = %q, want no cross-level suggestion", reason)
	}
	if strings.Contains(reason, "did you mean") {
		t.Errorf("must not suggest top-level key for nested field, got %q", reason)
	}
}

func TestSetRawValidator_UnsetNoChange(t *testing.T) {
	// Existing Load path with no SetRawValidator — smoke that basic load still works.
	type cfg struct {
		Port int `cfg:"port" validate:"required"`
	}
	got, err := New[cfg](mapSource{
		name: "t",
		data: map[string]any{"port": 8080},
	}).Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Port != 8080 {
		t.Errorf("Port = %d, want 8080", got.Port)
	}
}
