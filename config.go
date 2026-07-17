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

// Loader is the main entry point for loading, validating, and optionally
// hot-reloading a config struct of type T from one or more Sources.
// Construct with New (reflection bind/validate) or NewGenerated (codegen).
type Loader[T any] struct {
	sources []Source

	mu       sync.RWMutex
	current  *T
	watchCtx context.Context
	cancel   context.CancelFunc
	stops    []func() error

	onReload     []func(old, newCfg *T)
	onError      []func(error)
	stopWatchWG  sync.WaitGroup
	rawValidator func(map[string]any) error
	logger       *slog.Logger

	// binder, when non-nil, replaces reflection bind/validate (see NewGenerated).
	binder GeneratedBinder[T]
}

// New returns a Loader that binds and validates T via reflection.
// Sources are read in order on each Load; later sources override earlier
// ones for conflicting keys (including nested maps via recursive merge).
func New[T any](sources ...Source) *Loader[T] {
	return &Loader[T]{sources: sources}
}

// Load merges every Source (in order), optionally runs SetRawValidator on
// the merged map, then binds and validates into a new T. On success it
// stores that value for Get and returns it. On failure it returns
// *SourceError, *SchemaError, or *ValidationError and leaves any previously
// successful config untouched so Get keeps serving the last good value.
func (l *Loader[T]) Load(ctx context.Context) (*T, error) {
	merged := map[string]any{}
	for _, src := range l.sources {
		data, err := src.Load(ctx)
		if err != nil {
			return nil, err
		}
		merged = mergeMaps(merged, data)
	}

	l.mu.RLock()
	rawValidator := l.rawValidator
	l.mu.RUnlock()
	if rawValidator != nil {
		if err := rawValidator(merged); err != nil {
			return nil, &SchemaError{Err: err}
		}
	}

	var (
		cfg       *T
		bindErrs  []*FieldError
		setFields map[string]struct{}
	)
	if l.binder != nil {
		cfg, setFields, bindErrs = l.binder.BindGenerated(merged)
	} else {
		cfg = new(T)
		bindErrs, setFields = bind(cfg, merged)
	}
	if len(bindErrs) > 0 {
		return nil, &ValidationError{Errors: bindErrs}
	}
	var errs []*FieldError
	if l.binder != nil {
		errs = l.binder.ValidateGenerated(cfg, setFields, merged)
	} else {
		errs = validate(cfg, setFields, merged)
	}
	if len(errs) > 0 {
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
func (l *Loader[T]) OnReload(fn func(old, newCfg *T)) {
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

// SetRawValidator registers fn to run on the merged raw map after all
// sources succeed and before bind/validate (reflection or generated).
// A non-nil error from fn aborts Load as *SchemaError without binding.
// Passing nil clears the hook. Unset, Load skips this step entirely.
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

// Watch starts background watchers for every Source that implements
// Watchable. When any watcher signals a change, Load is re-run: success
// updates Get and fires OnReload (and SetLogger Info with Diff); failure
// fires OnError (and SetLogger Error) while Get continues returning the
// previous good config — including transient source I/O errors mid-edit.
// Watch itself returns immediately after starting goroutines; use Stop or
// cancel ctx to shut them down. Non-Watchable sources are still re-read
// on each reload but do not trigger one.
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
	onReload := append([]func(old, newCfg *T){}, l.onReload...)
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
