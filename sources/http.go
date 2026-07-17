package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/sinashahoveisi/typecfg"
	"gopkg.in/yaml.v3"
)

const (
	defaultPollInterval = 30 * time.Second
	httpErrorBodyCap    = 200
)

// RemoteHTTPSource loads config from an HTTP(S) endpoint serving JSON or
// YAML. It implements Watchable via polling (there is no push mechanism).
type RemoteHTTPSource struct {
	// URL is the absolute HTTP(S) endpoint to GET.
	URL string
	// Client is the HTTP client used for requests; nil means http.DefaultClient.
	Client *http.Client
	// Headers are extra request headers (e.g. Authorization) sent on each GET.
	Headers map[string]string
	// PollInterval is the Watch poll period; zero means 30s.
	PollInterval time.Duration
	// Format is "json" or "yaml". Empty means infer from Content-Type,
	// then URL path extension; if neither works, Load returns an error.
	Format string

	mu   sync.Mutex
	last map[string]any // snapshot of most recent successful Load
}

// NewRemoteHTTPSource returns a RemoteHTTPSource for the given URL.
func NewRemoteHTTPSource(rawURL string) *RemoteHTTPSource {
	return &RemoteHTTPSource{URL: rawURL}
}

// Name returns "http:<URL>" for SourceError messages.
func (s *RemoteHTTPSource) Name() string { return "http:" + s.URL }

func (s *RemoteHTTPSource) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return http.DefaultClient
}

// Load GETs URL, parses JSON or YAML into map[string]any, and stores a
// private snapshot used by Watch for change detection.
func (s *RemoteHTTPSource) Load(ctx context.Context) (map[string]any, error) {
	body, contentType, err := s.fetch(ctx)
	if err != nil {
		return nil, err
	}
	format, err := s.resolveFormat(contentType)
	if err != nil {
		return nil, &typecfg.SourceError{Source: s.Name(), Op: "parse", Err: err}
	}
	raw, err := parseHTTPBody(body, format)
	if err != nil {
		return nil, &typecfg.SourceError{Source: s.Name(), Op: "parse", Err: err}
	}
	// Remember a private clone so mergeMaps / callers cannot mutate the
	// Watch change-detection baseline, and so Watch can detect changes that
	// occurred between Load() and Watch() (the common Load-then-Watch pattern).
	s.mu.Lock()
	s.last = cloneMap(raw)
	s.mu.Unlock()
	return raw, nil
}

func (s *RemoteHTTPSource) fetch(ctx context.Context) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return nil, "", &typecfg.SourceError{Source: s.Name(), Op: "fetch", Err: err}
	}
	for k, v := range s.Headers {
		req.Header.Set(k, v)
	}

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, "", &typecfg.SourceError{Source: s.Name(), Op: "fetch", Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, httpErrorBodyCap))
		return nil, "", &typecfg.SourceError{
			Source: s.Name(),
			Op:     "fetch",
			Err:    fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(snippet)),
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", &typecfg.SourceError{Source: s.Name(), Op: "fetch", Err: err}
	}
	return body, resp.Header.Get("Content-Type"), nil
}

func (s *RemoteHTTPSource) resolveFormat(contentType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s.Format)) {
	case "json":
		return "json", nil
	case "yaml":
		return "yaml", nil
	case "":
		// fall through to inference
	default:
		return "", fmt.Errorf("unsupported Format %q (want \"json\" or \"yaml\")", s.Format)
	}

	if f := formatFromContentType(contentType); f != "" {
		return f, nil
	}
	if f := formatFromURL(s.URL); f != "" {
		return f, nil
	}
	return "", fmt.Errorf("cannot determine config format from Content-Type or URL; set Format to \"json\" or \"yaml\"")
}

func formatFromContentType(contentType string) string {
	ct := strings.ToLower(contentType)
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = ct[:i]
	}
	ct = strings.TrimSpace(ct)
	switch ct {
	case "application/json", "text/json":
		return "json"
	case "application/yaml", "text/yaml", "application/x-yaml", "text/x-yaml":
		return "yaml"
	default:
		return ""
	}
}

func formatFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	switch strings.ToLower(path.Ext(u.Path)) {
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return ""
	}
}

func parseHTTPBody(body []byte, format string) (map[string]any, error) {
	var raw map[string]any
	switch format {
	case "json":
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, err
		}
	case "yaml":
		if err := yaml.Unmarshal(body, &raw); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
	return raw, nil
}

// Watch polls the URL at PollInterval. It signals changed only when the
// parsed map differs from the last successful Load (deep equality),
// including the first poll after Watch starts — so a change between
// Load() and Watch() still surfaces via the loader's OnReload path.
// Transient fetch errors are ignored until the next tick.
func (s *RemoteHTTPSource) Watch(ctx context.Context) (<-chan struct{}, func() error, error) {
	watchCtx, cancel := context.WithCancel(ctx)
	changed := make(chan struct{}, 1)

	interval := s.PollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}

	go func() {
		defer close(changed)
		defer cancel()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		poll := func() {
			s.mu.Lock()
			prev := s.last
			s.mu.Unlock()

			data, err := s.Load(watchCtx)
			if err != nil {
				return
			}
			// No prior successful Load (Watch without Load): establish
			// baseline via Load's store above; do not signal.
			if prev == nil {
				return
			}
			if reflect.DeepEqual(prev, data) {
				return
			}
			select {
			case changed <- struct{}{}:
			default:
			}
		}

		poll() // compare immediately against Load baseline if any

		for {
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
				poll()
			}
		}
	}()

	stop := func() error {
		cancel()
		return nil
	}
	return changed, stop, nil
}

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = cloneValue(v)
	}
	return out
}

func cloneValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		return cloneMap(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = cloneValue(e)
		}
		return out
	default:
		return v
	}
}

var _ typecfg.Source = (*RemoteHTTPSource)(nil)
var _ typecfg.Watchable = (*RemoteHTTPSource)(nil)
