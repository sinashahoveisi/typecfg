package typecfg

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"
)

type parityCase struct {
	name string
	raw  map[string]any
}

func parityCases() []parityCase {
	return []parityCase{
		{
			name: "valid_full",
			raw: map[string]any{
				"name":    "app",
				"enabled": true,
				"count":   10,
				"ratio":   1.5,
				"timeout": "2s",
				"start":   "2026-01-01T00:00:00Z",
				"day":     "2026-07-16",
				"tags":    "a, b",
				"ports":   "8080,9090",
				"rates":   []any{1.5, 2.0},
				"labels":  map[string]any{"env": "prod"},
				"mode":    "dev",
				"code":    "ABC",
				"home":    "https://example.com",
				"email":   "user@example.com",
				"token":   "a",
				"server":  map[string]any{"host": "127.0.0.1", "port": 8080},
			},
		},
		{
			name: "defaults_and_nested_required",
			raw: map[string]any{
				"count":  1,
				"mode":   "prod",
				"code":   "Z",
				"home":   "https://x.test/",
				"email":  "a@b.c",
				"token":  "b",
				"server": map[string]any{"port": 80},
			},
		},
		{
			name: "native_int_slice",
			raw: map[string]any{
				"count":  1,
				"ports":  []any{1, 2, 3},
				"mode":   "dev",
				"code":   "A",
				"home":   "https://example.com",
				"email":  "u@e.com",
				"token":  "c",
				"server": map[string]any{"port": 1},
			},
		},
		{
			name: "labels_json_string",
			raw: map[string]any{
				"count":  1,
				"labels": `{"env":"staging","team":"core"}`,
				"mode":   "dev",
				"code":   "A",
				"home":   "https://example.com",
				"email":  "u@e.com",
				"token":  "a",
				"server": map[string]any{"port": 1},
			},
		},
		{
			name: "required_missing_count",
			raw: map[string]any{
				"mode":   "dev",
				"code":   "A",
				"home":   "https://example.com",
				"email":  "u@e.com",
				"token":  "a",
				"server": map[string]any{"port": 1},
			},
		},
		{
			name: "required_did_you_mean_port",
			raw: map[string]any{
				"count":  1,
				"mode":   "dev",
				"code":   "A",
				"home":   "https://example.com",
				"email":  "u@e.com",
				"token":  "a",
				"server": map[string]any{"prot": 8080},
			},
		},
		{
			name: "type_mismatch_count",
			raw: map[string]any{
				"count":  "nope",
				"mode":   "dev",
				"code":   "A",
				"home":   "https://example.com",
				"email":  "u@e.com",
				"token":  "a",
				"server": map[string]any{"port": 1},
			},
		},
		{
			name: "oneof_violation",
			raw: map[string]any{
				"count":  1,
				"mode":   "staging",
				"code":   "A",
				"home":   "https://example.com",
				"email":  "u@e.com",
				"token":  "a",
				"server": map[string]any{"port": 1},
			},
		},
		{
			name: "oneof_did_you_mean",
			raw: map[string]any{
				"count":  1,
				"mode":   "prd",
				"code":   "A",
				"home":   "https://example.com",
				"email":  "u@e.com",
				"token":  "a",
				"server": map[string]any{"port": 1},
			},
		},
		{
			name: "regexp_failure",
			raw: map[string]any{
				"count":  1,
				"mode":   "dev",
				"code":   "abc",
				"home":   "https://example.com",
				"email":  "u@e.com",
				"token":  "a",
				"server": map[string]any{"port": 1},
			},
		},
		{
			name: "secret_oneof_redaction",
			raw: map[string]any{
				"count":  1,
				"mode":   "dev",
				"code":   "A",
				"home":   "https://example.com",
				"email":  "u@e.com",
				"token":  "xyz",
				"server": map[string]any{"port": 1},
			},
		},
		{
			name: "gt_failure",
			raw: map[string]any{
				"count":  -1,
				"mode":   "dev",
				"code":   "A",
				"home":   "https://example.com",
				"email":  "u@e.com",
				"token":  "a",
				"server": map[string]any{"port": 1},
			},
		},
		{
			name: "url_failure",
			raw: map[string]any{
				"count":  1,
				"mode":   "dev",
				"code":   "A",
				"home":   "not a url",
				"email":  "u@e.com",
				"token":  "a",
				"server": map[string]any{"port": 1},
			},
		},
		{
			name: "email_failure",
			raw: map[string]any{
				"count":  1,
				"mode":   "dev",
				"code":   "A",
				"home":   "https://example.com",
				"email":  "not-an-email",
				"token":  "a",
				"server": map[string]any{"port": 1},
			},
		},
		{
			name: "slice_bad_element",
			raw: map[string]any{
				"count":  1,
				"ports":  "1,two,3",
				"mode":   "dev",
				"code":   "A",
				"home":   "https://example.com",
				"email":  "u@e.com",
				"token":  "a",
				"server": map[string]any{"port": 1},
			},
		},
		{
			name: "time_bad_format",
			raw: map[string]any{
				"count":  1,
				"start":  "01/01/2026",
				"mode":   "dev",
				"code":   "A",
				"home":   "https://example.com",
				"email":  "u@e.com",
				"token":  "a",
				"server": map[string]any{"port": 1},
			},
		},
		{
			name: "labels_bad_json",
			raw: map[string]any{
				"count":  1,
				"labels": "not-json",
				"mode":   "dev",
				"code":   "A",
				"home":   "https://example.com",
				"email":  "u@e.com",
				"token":  "a",
				"server": map[string]any{"port": 1},
			},
		},
	}
}

func TestParity_ReflectionVsGenerated(t *testing.T) {
	for _, tc := range parityCases() {
		t.Run(tc.name, func(t *testing.T) {
			raw := cloneRaw(tc.raw)
			refLoader := New[ParityConfig](mapSource{name: "t", data: cloneRaw(raw)})
			genLoader := NewGenerated[ParityConfig](ParityConfigBinder{}, mapSource{name: "t", data: cloneRaw(raw)})

			refCfg, refErr := refLoader.Load(context.Background())
			genCfg, genErr := genLoader.Load(context.Background())

			assertParityErrors(t, refErr, genErr)
			if refErr != nil || genErr != nil {
				return
			}
			if !parityConfigsEqual(refCfg, genCfg) {
				t.Fatalf("bound configs differ\nref=%+v\ngen=%+v", refCfg, genCfg)
			}
		})
	}
}

func TestParity_SetRawValidator(t *testing.T) {
	raw := map[string]any{
		"count":  1,
		"mode":   "dev",
		"code":   "A",
		"home":   "https://example.com",
		"email":  "u@e.com",
		"token":  "a",
		"server": map[string]any{"port": 1},
	}
	hook := func(m map[string]any) error {
		if _, ok := m["count"]; !ok {
			return fmt.Errorf("missing count in raw")
		}
		return fmt.Errorf("schema boom")
	}
	ref := New[ParityConfig](mapSource{name: "t", data: cloneRaw(raw)})
	ref.SetRawValidator(hook)
	gen := NewGenerated[ParityConfig](ParityConfigBinder{}, mapSource{name: "t", data: cloneRaw(raw)})
	gen.SetRawValidator(hook)

	_, refErr := ref.Load(context.Background())
	_, genErr := gen.Load(context.Background())
	var refSE, genSE *SchemaError
	if !errors.As(refErr, &refSE) || !errors.As(genErr, &genSE) {
		t.Fatalf("want SchemaError on both; ref=%v gen=%v", refErr, genErr)
	}
	if refSE.Error() != genSE.Error() {
		t.Fatalf("SchemaError messages differ: %q vs %q", refSE.Error(), genSE.Error())
	}
}

func assertParityErrors(t *testing.T, refErr, genErr error) {
	t.Helper()
	if (refErr == nil) != (genErr == nil) {
		t.Fatalf("error presence mismatch: ref=%v gen=%v", refErr, genErr)
	}
	if refErr == nil {
		return
	}
	var refVE, genVE *ValidationError
	if errors.As(refErr, &refVE) && errors.As(genErr, &genVE) {
		assertFieldErrorsEqual(t, refVE.Errors, genVE.Errors)
		return
	}
	if refErr.Error() != genErr.Error() {
		t.Fatalf("errors differ:\nref=%v\ngen=%v", refErr, genErr)
	}
}

func assertFieldErrorsEqual(t *testing.T, a, b []*FieldError) {
	t.Helper()
	an := normalizeFieldErrors(a)
	bn := normalizeFieldErrors(b)
	if len(an) != len(bn) {
		t.Fatalf("FieldError count %d vs %d\nref=%v\ngen=%v", len(an), len(bn), formatFEs(a), formatFEs(b))
	}
	for i := range an {
		if an[i].Field != bn[i].Field || an[i].Tag != bn[i].Tag || an[i].Reason != bn[i].Reason {
			t.Fatalf("FieldError[%d] mismatch\nref=%+v\ngen=%+v", i, an[i], bn[i])
		}
		if !reflect.DeepEqual(an[i].Sources, bn[i].Sources) {
			t.Fatalf("FieldError[%d].Sources mismatch\nref=%v\ngen=%v", i, an[i].Sources, bn[i].Sources)
		}
	}
}

// normalizeFieldErrors sorts by Field,Tag,Reason so comparison is stable if
// collection order ever diverges. Reflection and generated paths currently
// emit the same struct-field order; sorting is defensive only.
func normalizeFieldErrors(errs []*FieldError) []*FieldError {
	out := append([]*FieldError(nil), errs...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Field != out[j].Field {
			return out[i].Field < out[j].Field
		}
		if out[i].Tag != out[j].Tag {
			return out[i].Tag < out[j].Tag
		}
		return out[i].Reason < out[j].Reason
	})
	return out
}

func formatFEs(errs []*FieldError) string {
	s := ""
	for _, e := range errs {
		s += fmt.Sprintf("{%s/%s:%s} ", e.Field, e.Tag, e.Reason)
	}
	return s
}

func parityConfigsEqual(a, b *ParityConfig) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Name != b.Name || a.Enabled != b.Enabled || a.Count != b.Count || a.Ratio != b.Ratio {
		return false
	}
	if a.Timeout != b.Timeout || !a.Start.Equal(b.Start) || !a.Day.Equal(b.Day) {
		return false
	}
	if !reflect.DeepEqual(a.Tags, b.Tags) || !reflect.DeepEqual(a.Ports, b.Ports) || !reflect.DeepEqual(a.Rates, b.Rates) {
		return false
	}
	if !reflect.DeepEqual(a.Labels, b.Labels) {
		return false
	}
	if a.Mode != b.Mode || a.Code != b.Code || a.Home != b.Home || a.Email != b.Email || a.Token != b.Token {
		return false
	}
	if a.Server.Host != b.Server.Host || a.Server.Port != b.Server.Port {
		return false
	}
	return true
}

func cloneRaw(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		switch vv := v.(type) {
		case map[string]any:
			out[k] = cloneRaw(vv)
		case []any:
			cp := make([]any, len(vv))
			copy(cp, vv)
			out[k] = cp
		default:
			out[k] = v
		}
	}
	return out
}

// Keep time import used for Equal in helpers if Start zero.
var _ = time.Time{}
