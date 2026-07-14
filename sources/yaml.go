package sources

import (
	"context"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/sinashahoveisi/typecfg"
	"gopkg.in/yaml.v3"
)

// YAMLFile is a Source that reads a YAML file from disk and can watch it
// for changes using fsnotify.
type YAMLFile struct {
	Path string
}

func NewYAMLFile(path string) *YAMLFile {
	return &YAMLFile{Path: path}
}

func (f *YAMLFile) Name() string { return "file:" + f.Path }

func (f *YAMLFile) Load(_ context.Context) (map[string]any, error) {
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, &typecfg.SourceError{Source: f.Name(), Op: "read", Err: err}
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, &typecfg.SourceError{Source: f.Name(), Op: "parse", Err: err}
	}
	return raw, nil
}

func (f *YAMLFile) Watch(ctx context.Context) (<-chan struct{}, func() error, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, &typecfg.SourceError{Source: f.Name(), Op: "watch", Err: err}
	}
	if err := watcher.Add(filepath.Dir(f.Path)); err != nil {
		watcher.Close()
		return nil, nil, &typecfg.SourceError{Source: f.Name(), Op: "watch", Err: err}
	}

	targetBase := filepath.Base(f.Path)
	changed := make(chan struct{}, 1)
	go func() {
		defer close(changed)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				// Editors often do write+rename+create on save; treat any
				// of those as "content may have changed" and let the
				// loader re-read + diff to decide if anything really did.
				if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 &&
					filepath.Base(ev.Name) == targetBase {
					select {
					case changed <- struct{}{}:
					default:
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				_ = err // surfaced via SourceError from the next Load call
			}
		}
	}()

	stop := func() error { return watcher.Close() }
	return changed, stop, nil
}

var _ typecfg.Source = (*YAMLFile)(nil)
var _ typecfg.Watchable = (*YAMLFile)(nil)
