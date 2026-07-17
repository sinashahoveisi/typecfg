package benchmarks

import (
	"context"
	"testing"

	"github.com/sinashahoveisi/typecfg"
)

// mapSource is a fixed in-memory Source (same idea as the root package's
// test helper) so benchmarks measure bind/validate, not I/O.
type mapSource struct {
	data map[string]any
}

func (m mapSource) Name() string { return "bench" }

func (m mapSource) Load(context.Context) (map[string]any, error) {
	return m.data, nil
}

// validParityRaw is a complete, valid input for typecfg.ParityConfig.
// ParityConfig (+ ParityConfigBinder) live in the root module so the
// happy-path and validation-failure benches reuse the same representative
// surface already covered by gen_parity_test.go — no duplicated struct.
func validParityRaw() map[string]any {
	return map[string]any{
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
	}
}

// invalidParityRaw triggers several FieldErrors at once (required nested
// port, oneof mode, regexp code, url, email).
func invalidParityRaw() map[string]any {
	return map[string]any{
		"count":  1,
		"mode":   "staging",
		"code":   "abc",
		"home":   "not a url",
		"email":  "not-an-email",
		"token":  "a",
		"server": map[string]any{"host": "x"},
	}
}

func validDeepRaw() map[string]any {
	return map[string]any{
		"top": "bench",
		"l1": map[string]any{
			"label": "outer",
			"l2": map[string]any{
				"flag": true,
				"l3": map[string]any{
					"name": "leaf",
					"port": 8080,
					"host": "127.0.0.1",
				},
			},
		},
	}
}

func largeSliceRaw(n int) map[string]any {
	ports := make([]any, n)
	for i := 0; i < n; i++ {
		ports[i] = i + 1
	}
	return map[string]any{"ports": ports}
}

func BenchmarkBind_Reflection(b *testing.B) {
	loader := typecfg.New[typecfg.ParityConfig](mapSource{data: validParityRaw()})
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := loader.Load(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBind_Generated(b *testing.B) {
	loader := typecfg.NewGenerated[typecfg.ParityConfig](typecfg.ParityConfigBinder{}, mapSource{data: validParityRaw()})
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := loader.Load(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBind_Reflection_Nested(b *testing.B) {
	loader := typecfg.New[DeepConfig](mapSource{data: validDeepRaw()})
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := loader.Load(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBind_Generated_Nested(b *testing.B) {
	loader := typecfg.NewGenerated[DeepConfig](DeepConfigBinder{}, mapSource{data: validDeepRaw()})
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := loader.Load(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBind_Reflection_ValidationFailure(b *testing.B) {
	loader := typecfg.New[typecfg.ParityConfig](mapSource{data: invalidParityRaw()})
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := loader.Load(ctx)
		if err == nil {
			b.Fatal("expected validation error")
		}
	}
}

func BenchmarkBind_Generated_ValidationFailure(b *testing.B) {
	loader := typecfg.NewGenerated[typecfg.ParityConfig](typecfg.ParityConfigBinder{}, mapSource{data: invalidParityRaw()})
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := loader.Load(ctx)
		if err == nil {
			b.Fatal("expected validation error")
		}
	}
}

func BenchmarkBind_Reflection_LargeSlice(b *testing.B) {
	loader := typecfg.New[LargeSliceConfig](mapSource{data: largeSliceRaw(1000)})
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := loader.Load(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBind_Generated_LargeSlice(b *testing.B) {
	loader := typecfg.NewGenerated[LargeSliceConfig](LargeSliceConfigBinder{}, mapSource{data: largeSliceRaw(1000)})
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := loader.Load(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
