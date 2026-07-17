// Package sources provides optional Watchable typecfg.Source implementations
// that depend on third-party libraries (fsnotify, YAML). The core typecfg
// module stays dependency-free; import this submodule for file and HTTP sources.
package sources

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/sinashahoveisi/typecfg"
)

// JSONFile is a Source that reads a JSON object from Path and watches the
// file's directory for Write/Create/Rename of that basename (atomic saves).
type JSONFile struct {
	// Path is the filesystem path of the JSON file.
	Path string
}

// NewJSONFile returns a JSONFile Source for path.
func NewJSONFile(path string) *JSONFile {
	return &JSONFile{Path: path}
}

// Name returns "file:<Path>" for SourceError messages.
func (f *JSONFile) Name() string { return "file:" + f.Path }

// Load reads and json.Unmarshals Path into map[string]any.
func (f *JSONFile) Load(_ context.Context) (map[string]any, error) {
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, &typecfg.SourceError{Source: f.Name(), Op: "read", Err: err}
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, &typecfg.SourceError{Source: f.Name(), Op: "parse", Err: err}
	}
	return raw, nil
}

// Watch watches the parent directory and signals when Path's basename is
// written, created, or renamed (editor atomic-save patterns).
func (f *JSONFile) Watch(ctx context.Context) (<-chan struct{}, func() error, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, nil, &typecfg.SourceError{Source: f.Name(), Op: "watch", Err: err}
	}
	if err := watcher.Add(filepath.Dir(f.Path)); err != nil {
		_ = watcher.Close()
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
				if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 &&
					filepath.Base(ev.Name) == targetBase {
					select {
					case changed <- struct{}{}:
					default:
					}
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	stop := func() error { return watcher.Close() }
	return changed, stop, nil
}

var _ typecfg.Source = (*JSONFile)(nil)
var _ typecfg.Watchable = (*JSONFile)(nil)
