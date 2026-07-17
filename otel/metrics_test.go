package otel_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/sinashahoveisi/typecfg"
	typecfgotel "github.com/sinashahoveisi/typecfg/otel"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type watchableMap struct {
	name string
	mu   sync.Mutex
	data map[string]any
	ch   chan struct{}
}

func newWatchableMap(name string, data map[string]any) *watchableMap {
	return &watchableMap{name: name, data: data, ch: make(chan struct{}, 1)}
}

func (w *watchableMap) Name() string { return w.name }

func (w *watchableMap) Load(_ context.Context) (map[string]any, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make(map[string]any, len(w.data))
	for k, v := range w.data {
		out[k] = v
	}
	return out, nil
}

func (w *watchableMap) Watch(ctx context.Context) (<-chan struct{}, func() error, error) {
	return w.ch, func() error { return nil }, nil
}

func (w *watchableMap) update(data map[string]any) {
	w.mu.Lock()
	w.data = data
	w.mu.Unlock()
	select {
	case w.ch <- struct{}{}:
	default:
	}
}

type portConfig struct {
	Port int `cfg:"port" validate:"required,min=1"`
}

func counterSum(t *testing.T, rm *metricdata.ResourceMetrics, name string) int64 {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("%s: unexpected data type %T", name, m.Data)
			}
			var total int64
			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
			return total
		}
	}
	return 0
}

func TestRegister_ReloadIncrementsTotal(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	src := newWatchableMap("t", map[string]any{"port": 8080})
	loader := typecfg.New[portConfig](src)
	if _, err := loader.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := typecfgotel.Register(loader, provider.Meter("test")); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := loader.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	defer loader.Stop()

	src.update(map[string]any{"port": 9090})
	deadline := time.After(2 * time.Second)
	for {
		var rm metricdata.ResourceMetrics
		if err := reader.Collect(context.Background(), &rm); err != nil {
			t.Fatal(err)
		}
		if counterSum(t, &rm, "config_reload_total") >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for config_reload_total")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestRegister_ErrorIncrementsErrorsTotal(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	src := newWatchableMap("t", map[string]any{"port": 8080})
	loader := typecfg.New[portConfig](src)
	if _, err := loader.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := typecfgotel.Register(loader, provider.Meter("test")); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := loader.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	defer loader.Stop()

	src.update(map[string]any{}) // validation failure
	deadline := time.After(2 * time.Second)
	for {
		var rm metricdata.ResourceMetrics
		if err := reader.Collect(context.Background(), &rm); err != nil {
			t.Fatal(err)
		}
		if counterSum(t, &rm, "config_reload_errors_total") >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for config_reload_errors_total")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestRegister_CountersAccumulate(t *testing.T) {
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	src := newWatchableMap("t", map[string]any{"port": 8080})
	loader := typecfg.New[portConfig](src)
	if _, err := loader.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := typecfgotel.Register(loader, provider.Meter("test")); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := loader.Watch(ctx); err != nil {
		t.Fatal(err)
	}
	defer loader.Stop()

	waitAtLeast := func(name string, n int64) {
		t.Helper()
		deadline := time.After(2 * time.Second)
		for {
			var rm metricdata.ResourceMetrics
			if err := reader.Collect(context.Background(), &rm); err != nil {
				t.Fatal(err)
			}
			if counterSum(t, &rm, name) >= n {
				return
			}
			select {
			case <-deadline:
				t.Fatalf("timed out waiting for %s >= %d (got %d)", name, n, counterSum(t, &rm, name))
			case <-time.After(20 * time.Millisecond):
			}
		}
	}

	src.update(map[string]any{"port": 9090})
	waitAtLeast("config_reload_total", 1)
	src.update(map[string]any{"port": 9091})
	waitAtLeast("config_reload_total", 2)
	src.update(map[string]any{})
	waitAtLeast("config_reload_errors_total", 1)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	okTotal := counterSum(t, &rm, "config_reload_total")
	errTotal := counterSum(t, &rm, "config_reload_errors_total")
	if okTotal < 2 || errTotal < 1 {
		t.Fatalf("reload_total=%d errors_total=%d, want >=2 and >=1", okTotal, errTotal)
	}
}
