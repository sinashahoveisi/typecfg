// Package otel wires typecfg.Loader hot-reload hooks to OpenTelemetry
// metrics. It lives in a separate module so the core typecfg module stays
// free of otel dependencies.
package otel

import (
	"context"
	"fmt"

	"github.com/sinashahoveisi/typecfg"
	"go.opentelemetry.io/otel/metric"
)

// Register attaches OpenTelemetry counters to loader hot-reload callbacks:
//   - config_reload_total on each successful OnReload
//   - config_reload_errors_total on each OnError
//
// Counter Add calls use context.Background() because OnReload/OnError do
// not receive a request context.
func Register[T any](l *typecfg.Loader[T], meter metric.Meter) error {
	if l == nil {
		return fmt.Errorf("otel: loader is nil")
	}
	if meter == nil {
		return fmt.Errorf("otel: meter is nil")
	}

	reloads, err := meter.Int64Counter(
		"config_reload_total",
		metric.WithDescription("Successful typecfg hot-reload count"),
	)
	if err != nil {
		return fmt.Errorf("otel: create config_reload_total: %w", err)
	}
	errors, err := meter.Int64Counter(
		"config_reload_errors_total",
		metric.WithDescription("Failed typecfg hot-reload attempt count"),
	)
	if err != nil {
		return fmt.Errorf("otel: create config_reload_errors_total: %w", err)
	}

	l.OnReload(func(old, new *T) {
		reloads.Add(context.Background(), 1)
	})
	l.OnError(func(err error) {
		errors.Add(context.Background(), 1)
	})
	return nil
}
