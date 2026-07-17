// Package typecfg is a small, type-safe config loader for Go with
// first-class hot reload and error messages that tell you exactly which
// field is wrong and where it should have come from.
//
// Basic usage:
//
//	type Config struct {
//		Server struct {
//			Port int    `cfg:"port" validate:"required,min=1,max=65535"`
//			Host string `cfg:"host" default:"0.0.0.0"`
//		} `cfg:"server"`
//	}
//
//	loader := typecfg.New[Config](
//		sources.NewYAMLFile("config.yaml"),
//		typecfg.NewEnvSource("APP"),
//	)
//	cfg, err := loader.Load(ctx)
//
// Sources are applied in order, later sources overriding earlier ones, so
// putting the env source last means env vars override the YAML file --
// the usual convention for container deployments.
package typecfg

import (
	"context"
	"log/slog"
	"sync"
)

// Loader loads a config struct of type T from one or more Sources,
// validates it, and optionally watches sources for changes.
type Loader[T any] struct {
	sources []Source

	mu       sync.RWMutex
	current  *T
	watchCtx context.Context
	cancel   context.CancelFunc
	stops    []func() error

	onReload     []func(old, new *T)
	onError      []func(error)
	stopWatchWG  sync.WaitGroup
	rawValidator func(map[string]any) error
	logger       *slog.Logger
}

// New creates a Loader that reads from the given sources, in order.
// Later sources override earlier ones for any key they both define.
func New[T any](sources ...Source) *Loader[T] {
	return &Loader[T]{sources: sources}
}

// Load reads all sources, binds them onto a new T, validates it, and — on
// success — stores it as the current config (retrievable via Get).
// On validation or parsing failure, the previously loaded config (if any)
// is left untouched and a *ValidationError or *SourceError is returned.
func (l *Loader[T]) Load(ctx context.Context) (*T, error) {
	merged := map[string]any{}
	for _, src := range l.sources {
		data, err := src.Load(ctx)
		if err != nil {
			return nil, err
		}
		merged = mergeMaps(merged, data)
	}

	cfg := new(T)
	l.mu.RLock()
	rawValidator := l.rawValidator
	l.mu.RUnlock()
	if rawValidator != nil {
		if err := rawValidator(merged); err != nil {
			return nil, &SchemaError{Err: err}
		}
	}
	bindErrs, setFields := bind(cfg, merged)
	if len(bindErrs) > 0 {
		return nil, &ValidationError{Errors: bindErrs}
	}
	if errs := validate(cfg, setFields, merged); len(errs) > 0 {
		return nil, &ValidationError{Errors: errs}
	}

	l.mu.Lock()
	l.current = cfg
	l.mu.Unlock()

	return cfg, nil
}

// Get returns the most recently successfully loaded config. It is safe to
// call concurrently with Watch's background reloads. Returns nil if Load
// has never succeeded.
func (l *Loader[T]) Get() *T {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.current
}

// OnReload registers a callback invoked after a successful hot-reload
// picks up a new, valid config. It is NOT called for the initial Load.
func (l *Loader[T]) OnReload(fn func(old, new *T)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onReload = append(l.onReload, fn)
}

// OnError registers a callback invoked when a hot-reload attempt fails
// (e.g. the file was saved mid-write, or a validation rule now fails).
// The previously loaded config keeps being served by Get in that case --
// a bad edit never leaves your service without a config.
func (l *Loader[T]) OnError(fn func(error)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onError = append(l.onError, fn)
}

// SetRawValidator registers an optional hook invoked with the merged raw
// config map after sources are loaded and merged, but before bind and
// struct-tag validation. Use it for opt-in checks such as JSON Schema
// (see the jsonschema/ submodule). If fn is nil, the hook is cleared.
// When unset, Load behaves exactly as before.
func (l *Loader[T]) SetRawValidator(fn func(map[string]any) error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rawValidator = fn
}

// SetLogger registers an optional slog.Logger used by hot-reload to emit
// structured logs on success (Info) and failure (Error). Logging is
// additive alongside OnReload/OnError callbacks. If logger is nil, or
// SetLogger is never called, reload behavior is unchanged.
func (l *Loader[T]) SetLogger(logger *slog.Logger) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logger = logger
}

// Watch starts watching all Watchable sources in the background. Every
// time any of them signals a change, all sources are re-read, re-bound,
// and re-validated; on success Get() starts returning the new value and
// OnReload callbacks fire; on failure OnError callbacks fire and the old
// config keeps being served.
//
// Watch returns immediately; call Stop (or cancel ctx) to stop watching.
func (l *Loader[T]) Watch(ctx context.Context) error {
	watchCtx, cancel := context.WithCancel(ctx)
	l.watchCtx = watchCtx
	l.cancel = cancel

	changedAgg := make(chan struct{}, 1)
	for _, src := range l.sources {
		w, ok := src.(Watchable)
		if !ok {
			continue
		}
		ch, stop, err := w.Watch(watchCtx)
		if err != nil {
			cancel()
			return err
		}
		l.stops = append(l.stops, stop)

		l.stopWatchWG.Add(1)
		go func(ch <-chan struct{}) {
			defer l.stopWatchWG.Done()
			for {
				select {
				case <-watchCtx.Done():
					return
				case _, ok := <-ch:
					if !ok {
						return
					}
					select {
					case changedAgg <- struct{}{}:
					default:
					}
				}
			}
		}(ch)
	}

	l.stopWatchWG.Add(1)
	go func() {
		defer l.stopWatchWG.Done()
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-changedAgg:
				l.reload(watchCtx)
			}
		}
	}()

	return nil
}

func (l *Loader[T]) reload(ctx context.Context) {
	old := l.Get()
	newCfg, err := l.Load(ctx)

	l.mu.RLock()
	onReload := append([]func(old, new *T){}, l.onReload...)
	onError := append([]func(error){}, l.onError...)
	logger := l.logger
	l.mu.RUnlock()

	if err != nil {
		if logger != nil {
			logger.Error("config reload failed", slog.Any("err", err))
		}
		for _, fn := range onError {
			fn(err)
		}
		return
	}
	if logger != nil {
		changes := Diff(old, newCfg)
		attrs := make([]any, 0, 1+len(changes))
		attrs = append(attrs, slog.Int("change_count", len(changes)))
		for _, c := range changes {
			attrs = append(attrs, slog.Group(c.Path,
				slog.Any("old", c.Old),
				slog.Any("new", c.New),
			))
		}
		logger.Info("config reloaded", attrs...)
	}
	for _, fn := range onReload {
		fn(old, newCfg)
	}
}

// Stop stops all source watchers started by Watch and waits for their
// goroutines to exit. Safe to call even if Watch was never called.
func (l *Loader[T]) Stop() error {
	if l.cancel != nil {
		l.cancel()
	}
	var firstErr error
	for _, stop := range l.stops {
		if err := stop(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	l.stopWatchWG.Wait()
	return firstErr
}
