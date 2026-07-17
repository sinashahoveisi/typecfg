package sources

import (
	"context"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/sinashahoveisi/typecfg"
	"gopkg.in/yaml.v3"
)

// YAMLFile is a Source that reads a YAML mapping from Path and watches the
// file's directory for Write/Create/Rename of that basename (atomic saves).
type YAMLFile struct {
	// Path is the filesystem path of the YAML file.
	Path string
}

// NewYAMLFile returns a YAMLFile Source for path.
func NewYAMLFile(path string) *YAMLFile {
	return &YAMLFile{Path: path}
}

// Name returns "file:<Path>" for SourceError messages.
func (f *YAMLFile) Name() string { return "file:" + f.Path }

// Load reads and yaml.Unmarshals Path into map[string]any.
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

// Watch watches the parent directory and signals when Path's basename is
// written, created, or renamed (editor atomic-save patterns).
func (f *YAMLFile) Watch(ctx context.Context) (<-chan struct{}, func() error, error) {
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
