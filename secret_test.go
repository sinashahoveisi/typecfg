package typecfg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSecret_BindTypeErrorRedactsValue(t *testing.T) {
	const bad = "not-a-number-SECRET-xyz"
	type cfg struct {
		Token int `cfg:"token" secret:"true"`
	}
	_, err := New[cfg](mapSource{
		name: "t",
		data: map[string]any{"token": bad},
	}).Load(context.Background())
	if err == nil {
		t.Fatal("expected bind type error")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) || len(verr.Errors) == 0 {
		t.Fatalf("got %T: %v", err, err)
	}
	reason := verr.Errors[0].Reason
	msg := verr.Error()
	if !strings.Contains(reason, redactedMarker) {
		t.Errorf("Reason should contain %q, got %q", redactedMarker, reason)
	}
	if strings.Contains(reason, bad) {
		t.Errorf("Reason must not contain raw value %q: %q", bad, reason)
	}
	if strings.Contains(msg, bad) {
		t.Errorf("ValidationError.Error() must not contain raw value %q: %q", bad, msg)
	}
}

func TestSecret_OneofRedactsValueAndSuppressesSuggestion(t *testing.T) {
	const bad = "xyz"
	type cfg struct {
		Mode string `cfg:"mode" secret:"true" validate:"oneof=a b c"`
	}
	_, err := New[cfg](mapSource{
		name: "t",
		data: map[string]any{"mode": bad},
	}).Load(context.Background())
	if err == nil {
		t.Fatal("expected oneof error")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) || len(verr.Errors) == 0 {
		t.Fatalf("got %T: %v", err, err)
	}
	reason := verr.Errors[0].Reason
	want := `must be one of [a b c], got "***REDACTED***"`
	if reason != want {
		t.Errorf("Reason = %q, want %q", reason, want)
	}
	if strings.Contains(reason, bad) {
		t.Errorf("Reason must not contain %q", bad)
	}
	if strings.Contains(reason, "did you mean") {
		t.Errorf("secret oneof must not suggest, got %q", reason)
	}
	// Options stay visible.
	for _, opt := range []string{"a", "b", "c"} {
		if !strings.Contains(reason, opt) {
			t.Errorf("options must stay visible; missing %q in %q", opt, reason)
		}
	}
}

func TestSecret_OneofCloseValueNoSuggestion(t *testing.T) {
	// "ifo" is distance 1 from "info" — would suggest without secret.
	const bad = "ifo"
	type cfg struct {
		Level string `cfg:"level" secret:"true" validate:"oneof=debug info warn error"`
	}
	_, err := New[cfg](mapSource{
		name: "t",
		data: map[string]any{"level": bad},
	}).Load(context.Background())
	if err == nil {
		t.Fatal("expected oneof error")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) || len(verr.Errors) == 0 {
		t.Fatalf("got %T: %v", err, err)
	}
	reason := verr.Errors[0].Reason
	if strings.Contains(reason, bad) {
		t.Errorf("must not contain value %q: %q", bad, reason)
	}
	if strings.Contains(reason, "did you mean") {
		t.Errorf("must not suggest for secret: %q", reason)
	}
	if !strings.Contains(reason, "info") {
		t.Errorf("allowed options must remain visible: %q", reason)
	}
}

func TestSecret_RegexpRedactsValueKeepsPattern(t *testing.T) {
	const bad = "token-leak-VALUE-99"
	const pattern = `^sk_[a-z]+$`
	type cfg struct {
		Key string `cfg:"key" secret:"true" validate:"regexp=^sk_[a-z]+$"`
	}
	_, err := New[cfg](mapSource{
		name: "t",
		data: map[string]any{"key": bad},
	}).Load(context.Background())
	if err == nil {
		t.Fatal("expected regexp error")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) || len(verr.Errors) == 0 {
		t.Fatalf("got %T: %v", err, err)
	}
	reason := verr.Errors[0].Reason
	want := fmt.Sprintf(`must match pattern %q, got %q`, pattern, redactedMarker)
	if reason != want {
		t.Errorf("Reason = %q, want %q", reason, want)
	}
	if strings.Contains(reason, bad) {
		t.Errorf("must not contain value %q", bad)
	}
	if !strings.Contains(reason, pattern) {
		t.Errorf("pattern must not be redacted: %q", reason)
	}
}

func TestSecret_NumericRulesRedactValueKeepLimit(t *testing.T) {
	cases := []struct {
		name   string
		value  int
		limit  string
		prefix string
	}{
		{name: "min", value: 3, limit: "10", prefix: "must be >= 10"},
		{name: "max", value: 200, limit: "100", prefix: "must be <= 100"},
		{name: "gt", value: 5, limit: "10", prefix: "must be > 10"},
		{name: "lt", value: 150, limit: "100", prefix: "must be < 100"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			switch tc.name {
			case "min":
				type c struct {
					N int `cfg:"n" secret:"true" validate:"min=10"`
				}
				_, err = New[c](mapSource{name: "t", data: map[string]any{"n": tc.value}}).Load(context.Background())
			case "max":
				type c struct {
					N int `cfg:"n" secret:"true" validate:"max=100"`
				}
				_, err = New[c](mapSource{name: "t", data: map[string]any{"n": tc.value}}).Load(context.Background())
			case "gt":
				type c struct {
					N int `cfg:"n" secret:"true" validate:"gt=10"`
				}
				_, err = New[c](mapSource{name: "t", data: map[string]any{"n": tc.value}}).Load(context.Background())
			case "lt":
				type c struct {
					N int `cfg:"n" secret:"true" validate:"lt=100"`
				}
				_, err = New[c](mapSource{name: "t", data: map[string]any{"n": tc.value}}).Load(context.Background())
			}
			if err == nil {
				t.Fatal("expected validation error")
			}
			var verr *ValidationError
			if !errors.As(err, &verr) || len(verr.Errors) == 0 {
				t.Fatalf("got %T: %v", err, err)
			}
			reason := verr.Errors[0].Reason
			want := fmt.Sprintf("%s, got %s", tc.prefix, redactedMarker)
			if reason != want {
				t.Errorf("Reason = %q, want %q", reason, want)
			}
			if !strings.Contains(reason, tc.limit) {
				t.Errorf("limit %q must stay visible in %q", tc.limit, reason)
			}
			gotStr := fmt.Sprintf("%d", tc.value)
			if strings.Contains(reason, gotStr) {
				t.Errorf("Reason must not contain failing value %q: %q", gotStr, reason)
			}
		})
	}
}

func TestSecret_URLRedactsValue(t *testing.T) {
	const bad = "not a url SECRET-leak"
	type cfg struct {
		Endpoint string `cfg:"endpoint" secret:"true" validate:"url"`
	}
	_, err := New[cfg](mapSource{
		name: "t",
		data: map[string]any{"endpoint": bad},
	}).Load(context.Background())
	if err == nil {
		t.Fatal("expected url error")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) || len(verr.Errors) == 0 {
		t.Fatalf("got %T: %v", err, err)
	}
	reason := verr.Errors[0].Reason
	want := fmt.Sprintf("must be a valid URL, got %q", redactedMarker)
	if reason != want {
		t.Errorf("Reason = %q, want %q", reason, want)
	}
	if strings.Contains(reason, bad) {
		t.Errorf("Reason must not contain %q: %q", bad, reason)
	}
}

func TestSecret_EmailRedactsValue(t *testing.T) {
	const bad = "not-an-email-SECRET-leak"
	type cfg struct {
		Addr string `cfg:"addr" secret:"true" validate:"email"`
	}
	_, err := New[cfg](mapSource{
		name: "t",
		data: map[string]any{"addr": bad},
	}).Load(context.Background())
	if err == nil {
		t.Fatal("expected email error")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) || len(verr.Errors) == 0 {
		t.Fatalf("got %T: %v", err, err)
	}
	reason := verr.Errors[0].Reason
	want := fmt.Sprintf("must be a valid email address, got %q", redactedMarker)
	if reason != want {
		t.Errorf("Reason = %q, want %q", reason, want)
	}
	if strings.Contains(reason, bad) {
		t.Errorf("Reason must not contain %q: %q", bad, reason)
	}
}

func TestSecret_NonSecretUnchanged(t *testing.T) {
	t.Run("bind", func(t *testing.T) {
		const bad = "not-a-number"
		type cfg struct {
			Port int `cfg:"port"`
		}
		_, err := New[cfg](mapSource{
			name: "t",
			data: map[string]any{"port": bad},
		}).Load(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) || len(verr.Errors) == 0 {
			t.Fatalf("got %T: %v", err, err)
		}
		reason := verr.Errors[0].Reason
		if strings.Contains(reason, redactedMarker) {
			t.Errorf("non-secret must not redact: %q", reason)
		}
		if !strings.Contains(reason, bad) {
			t.Errorf("non-secret should quote bad input, got %q", reason)
		}
	})

	t.Run("oneof", func(t *testing.T) {
		type cfg struct {
			Level string `cfg:"level" validate:"oneof=debug info warn error"`
		}
		_, err := New[cfg](mapSource{
			name: "t",
			data: map[string]any{"level": "ifno"},
		}).Load(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) || len(verr.Errors) == 0 {
			t.Fatalf("got %T: %v", err, err)
		}
		want := `must be one of [debug info warn error], got "ifno" (did you mean "info"?)`
		if verr.Errors[0].Reason != want {
			t.Errorf("Reason = %q, want %q", verr.Errors[0].Reason, want)
		}
	})

	t.Run("regexp", func(t *testing.T) {
		const bad = "nope"
		const pattern = `^sk_[a-z]+$`
		type cfg struct {
			Key string `cfg:"key" validate:"regexp=^sk_[a-z]+$"`
		}
		_, err := New[cfg](mapSource{
			name: "t",
			data: map[string]any{"key": bad},
		}).Load(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) || len(verr.Errors) == 0 {
			t.Fatalf("got %T: %v", err, err)
		}
		want := fmt.Sprintf(`must match pattern %q, got %q`, pattern, bad)
		if verr.Errors[0].Reason != want {
			t.Errorf("Reason = %q, want %q", verr.Errors[0].Reason, want)
		}
	})

	t.Run("min", func(t *testing.T) {
		type cfg struct {
			N int `cfg:"n" validate:"min=10"`
		}
		_, err := New[cfg](mapSource{name: "t", data: map[string]any{"n": 3}}).Load(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) || len(verr.Errors) == 0 {
			t.Fatalf("got %T: %v", err, err)
		}
		want := "must be >= 10, got 3"
		if verr.Errors[0].Reason != want {
			t.Errorf("Reason = %q, want %q", verr.Errors[0].Reason, want)
		}
	})

	t.Run("max", func(t *testing.T) {
		type cfg struct {
			N int `cfg:"n" validate:"max=100"`
		}
		_, err := New[cfg](mapSource{name: "t", data: map[string]any{"n": 200}}).Load(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) || len(verr.Errors) == 0 {
			t.Fatalf("got %T: %v", err, err)
		}
		want := "must be <= 100, got 200"
		if verr.Errors[0].Reason != want {
			t.Errorf("Reason = %q, want %q", verr.Errors[0].Reason, want)
		}
	})

	t.Run("gt", func(t *testing.T) {
		type cfg struct {
			N int `cfg:"n" validate:"gt=10"`
		}
		_, err := New[cfg](mapSource{name: "t", data: map[string]any{"n": 5}}).Load(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) || len(verr.Errors) == 0 {
			t.Fatalf("got %T: %v", err, err)
		}
		want := "must be > 10, got 5"
		if verr.Errors[0].Reason != want {
			t.Errorf("Reason = %q, want %q", verr.Errors[0].Reason, want)
		}
	})

	t.Run("lt", func(t *testing.T) {
		type cfg struct {
			N int `cfg:"n" validate:"lt=100"`
		}
		_, err := New[cfg](mapSource{name: "t", data: map[string]any{"n": 150}}).Load(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) || len(verr.Errors) == 0 {
			t.Fatalf("got %T: %v", err, err)
		}
		want := "must be < 100, got 150"
		if verr.Errors[0].Reason != want {
			t.Errorf("Reason = %q, want %q", verr.Errors[0].Reason, want)
		}
	})

	t.Run("url", func(t *testing.T) {
		const bad = "not a url SECRET-leak"
		type cfg struct {
			Endpoint string `cfg:"endpoint" validate:"url"`
		}
		_, err := New[cfg](mapSource{
			name: "t",
			data: map[string]any{"endpoint": bad},
		}).Load(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) || len(verr.Errors) == 0 {
			t.Fatalf("got %T: %v", err, err)
		}
		reason := verr.Errors[0].Reason
		if strings.Contains(reason, redactedMarker) {
			t.Errorf("non-secret must not redact: %q", reason)
		}
		if !strings.Contains(reason, bad) {
			t.Errorf("non-secret should include bad input, got %q", reason)
		}
		if !strings.HasPrefix(reason, fmt.Sprintf("must be a valid URL, got %q", bad)) {
			t.Errorf("Reason = %q, want prefix with bad URL", reason)
		}
	})

	t.Run("email", func(t *testing.T) {
		const bad = "not-an-email-SECRET-leak"
		type cfg struct {
			Addr string `cfg:"addr" validate:"email"`
		}
		_, err := New[cfg](mapSource{
			name: "t",
			data: map[string]any{"addr": bad},
		}).Load(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
		var verr *ValidationError
		if !errors.As(err, &verr) || len(verr.Errors) == 0 {
			t.Fatalf("got %T: %v", err, err)
		}
		reason := verr.Errors[0].Reason
		if strings.Contains(reason, redactedMarker) {
			t.Errorf("non-secret must not redact: %q", reason)
		}
		if !strings.Contains(reason, bad) {
			t.Errorf("non-secret should include bad input, got %q", reason)
		}
		if !strings.HasPrefix(reason, fmt.Sprintf("must be a valid email address, got %q", bad)) {
			t.Errorf("Reason = %q, want prefix with bad email", reason)
		}
	})
}

func TestSecret_ValidationErrorAggregationNoLeak(t *testing.T) {
	const secretVal = "super-secret-TOKEN-abc"
	type cfg struct {
		Token string `cfg:"token" secret:"true" validate:"oneof=a b c"`
		Port  int    `cfg:"port" validate:"required"`
	}
	_, err := New[cfg](mapSource{
		name: "t",
		data: map[string]any{"token": secretVal},
	}).Load(context.Background())
	if err == nil {
		t.Fatal("expected errors")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("got %T: %v", err, err)
	}
	if len(verr.Errors) < 2 {
		t.Fatalf("expected multiple errors, got %+v", verr.Errors)
	}
	agg := verr.Error()
	if strings.Contains(agg, secretVal) {
		t.Errorf("aggregated Error() leaked secret: %q", agg)
	}
	// %+v on FieldError only exposes Reason (already redacted) — no separate
	// raw-value field. Document residual risk of formatting the *config*
	// struct itself, not FieldError.
	plusV := fmt.Sprintf("%+v", verr.Errors[0])
	if strings.Contains(plusV, secretVal) {
		t.Errorf("%%+v on FieldError leaked secret: %s", plusV)
	}
	if !strings.Contains(plusV, redactedMarker) {
		t.Errorf("%%+v should show redacted Reason, got %s", plusV)
	}
}
