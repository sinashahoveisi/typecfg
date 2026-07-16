package typecfg

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestValidate_GT(t *testing.T) {
	type cfg struct {
		N int `cfg:"n" validate:"gt=10"`
	}

	t.Run("pass_above", func(t *testing.T) {
		got, err := New[cfg](mapSource{name: "t", data: map[string]any{"n": 11}}).Load(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.N != 11 {
			t.Errorf("N = %d, want 11", got.N)
		}
	})

	t.Run("fail_at_limit", func(t *testing.T) {
		_, err := New[cfg](mapSource{name: "t", data: map[string]any{"n": 10}}).Load(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("got %T: %v", err, err)
		}
		want := "must be > 10, got 10"
		if verr.Errors[0].Reason != want {
			t.Errorf("Reason = %q, want %q", verr.Errors[0].Reason, want)
		}
	})

	t.Run("fail_below", func(t *testing.T) {
		_, err := New[cfg](mapSource{name: "t", data: map[string]any{"n": 9}}).Load(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("got %T: %v", err, err)
		}
		want := "must be > 10, got 9"
		if verr.Errors[0].Reason != want {
			t.Errorf("Reason = %q, want %q", verr.Errors[0].Reason, want)
		}
	})
}

func TestValidate_LT(t *testing.T) {
	type cfg struct {
		N int `cfg:"n" validate:"lt=100"`
	}

	t.Run("pass_below", func(t *testing.T) {
		got, err := New[cfg](mapSource{name: "t", data: map[string]any{"n": 99}}).Load(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.N != 99 {
			t.Errorf("N = %d, want 99", got.N)
		}
	})

	t.Run("fail_at_limit", func(t *testing.T) {
		_, err := New[cfg](mapSource{name: "t", data: map[string]any{"n": 100}}).Load(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("got %T: %v", err, err)
		}
		want := "must be < 100, got 100"
		if verr.Errors[0].Reason != want {
			t.Errorf("Reason = %q, want %q", verr.Errors[0].Reason, want)
		}
	})

	t.Run("fail_above", func(t *testing.T) {
		_, err := New[cfg](mapSource{name: "t", data: map[string]any{"n": 101}}).Load(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("got %T: %v", err, err)
		}
		want := "must be < 100, got 101"
		if verr.Errors[0].Reason != want {
			t.Errorf("Reason = %q, want %q", verr.Errors[0].Reason, want)
		}
	})
}

func TestValidate_Regexp(t *testing.T) {
	t.Run("pass_match", func(t *testing.T) {
		type cfg struct {
			Code string `cfg:"code" validate:"regexp=^[a-z]+$"`
		}
		got, err := New[cfg](mapSource{name: "t", data: map[string]any{"code": "abc"}}).Load(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Code != "abc" {
			t.Errorf("Code = %q, want abc", got.Code)
		}
	})

	t.Run("fail_non_match", func(t *testing.T) {
		type cfg struct {
			Code string `cfg:"code" validate:"regexp=^[a-z]+$"`
		}
		_, err := New[cfg](mapSource{name: "t", data: map[string]any{"code": "ABC"}}).Load(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("got %T: %v", err, err)
		}
		reason := verr.Errors[0].Reason
		t.Logf("reason: %s", reason)
		want := `must match pattern "^[a-z]+$", got "ABC"`
		if reason != want {
			t.Errorf("Reason = %q, want %q", reason, want)
		}
	})

	t.Run("invalid_pattern", func(t *testing.T) {
		type cfg struct {
			Code string `cfg:"code" validate:"regexp=["`
		}
		_, err := New[cfg](mapSource{name: "t", data: map[string]any{"code": "x"}}).Load(context.Background())
		if err == nil {
			t.Fatal("expected error for invalid regexp pattern")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("got %T: %v", err, err)
		}
		reason := verr.Errors[0].Reason
		t.Logf("reason: %s", reason)
		if !strings.Contains(reason, `invalid regexp pattern "["`) {
			t.Errorf("Reason = %q, want invalid pattern message", reason)
		}
	})

	t.Run("non_string_field", func(t *testing.T) {
		type cfg struct {
			N int `cfg:"n" validate:"regexp=^[0-9]+$"`
		}
		_, err := New[cfg](mapSource{name: "t", data: map[string]any{"n": 42}}).Load(context.Background())
		if err == nil {
			t.Fatal("expected type mismatch error")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("got %T: %v", err, err)
		}
		reason := verr.Errors[0].Reason
		t.Logf("reason: %s", reason)
		if !strings.Contains(reason, "only valid on string fields") {
			t.Errorf("Reason = %q, want string-only mismatch", reason)
		}
	})
}

func TestValidate_URL(t *testing.T) {
	t.Run("pass_absolute", func(t *testing.T) {
		type cfg struct {
			Home string `cfg:"home" validate:"url"`
		}
		got, err := New[cfg](mapSource{name: "t", data: map[string]any{"home": "https://example.com/path"}}).Load(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Home != "https://example.com/path" {
			t.Errorf("Home = %q", got.Home)
		}
	})

	t.Run("fail_garbage", func(t *testing.T) {
		type cfg struct {
			Home string `cfg:"home" validate:"url"`
		}
		_, err := New[cfg](mapSource{name: "t", data: map[string]any{"home": "not a url"}}).Load(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("got %T: %v", err, err)
		}
		reason := verr.Errors[0].Reason
		t.Logf("reason: %s", reason)
		if !strings.Contains(reason, `must be a valid URL, got "not a url"`) {
			t.Errorf("Reason = %q", reason)
		}
	})

	t.Run("fail_empty", func(t *testing.T) {
		type cfg struct {
			Home string `cfg:"home" validate:"url"`
		}
		_, err := New[cfg](mapSource{name: "t", data: map[string]any{"home": ""}}).Load(context.Background())
		if err == nil {
			t.Fatal("expected error for empty URL")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("got %T: %v", err, err)
		}
		reason := verr.Errors[0].Reason
		t.Logf("reason: %s", reason)
		if !strings.Contains(reason, `must be a valid URL, got ""`) {
			t.Errorf("Reason = %q", reason)
		}
	})

	t.Run("non_string_field", func(t *testing.T) {
		type cfg struct {
			N int `cfg:"n" validate:"url"`
		}
		_, err := New[cfg](mapSource{name: "t", data: map[string]any{"n": 1}}).Load(context.Background())
		if err == nil {
			t.Fatal("expected type mismatch error")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("got %T: %v", err, err)
		}
		reason := verr.Errors[0].Reason
		t.Logf("reason: %s", reason)
		if !strings.Contains(reason, "only valid on string fields") {
			t.Errorf("Reason = %q", reason)
		}
	})
}

func TestValidate_Email(t *testing.T) {
	t.Run("pass_valid", func(t *testing.T) {
		type cfg struct {
			Addr string `cfg:"addr" validate:"email"`
		}
		got, err := New[cfg](mapSource{name: "t", data: map[string]any{"addr": "user@example.com"}}).Load(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Addr != "user@example.com" {
			t.Errorf("Addr = %q", got.Addr)
		}
	})

	t.Run("fail_invalid", func(t *testing.T) {
		type cfg struct {
			Addr string `cfg:"addr" validate:"email"`
		}
		_, err := New[cfg](mapSource{name: "t", data: map[string]any{"addr": "not-an-email"}}).Load(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("got %T: %v", err, err)
		}
		reason := verr.Errors[0].Reason
		t.Logf("reason: %s", reason)
		if !strings.Contains(reason, `must be a valid email address, got "not-an-email"`) {
			t.Errorf("Reason = %q", reason)
		}
	})
}

func TestValidate_RequiredAndRegexpCompose(t *testing.T) {
	type cfg struct {
		Code string `cfg:"code" validate:"required,regexp=^[A-Z]"`
	}
	_, err := New[cfg](mapSource{name: "t", data: map[string]any{}}).Load(context.Background())
	if err == nil {
		t.Fatal("expected errors")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("got %T: %v", err, err)
	}
	var sawRequired, sawRegexp bool
	for _, e := range verr.Errors {
		t.Logf("error: tag=%s reason=%s", e.Tag, e.Reason)
		switch e.Tag {
		case "required":
			sawRequired = true
		case "regexp":
			sawRegexp = true
		}
	}
	if !sawRequired || !sawRegexp {
		t.Fatalf("expected both required and regexp errors, got %+v", verr.Errors)
	}
}
