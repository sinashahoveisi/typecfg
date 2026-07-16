package sources

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sinashahoveisi/typecfg"
)

func writeJSON(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestJSONFile_IntSliceNativeArray(t *testing.T) {
	type portsConfig struct {
		Ports []int `cfg:"ports"`
	}
	path := writeJSON(t, `{"ports":[8080,8443]}`)
	loader := typecfg.New[portsConfig](NewJSONFile(path))
	cfg, err := loader.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Ports) != 2 || cfg.Ports[0] != 8080 || cfg.Ports[1] != 8443 {
		t.Errorf("Ports = %v, want [8080 8443]", cfg.Ports)
	}
}
