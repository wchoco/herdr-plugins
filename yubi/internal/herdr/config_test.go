package herdr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[hint]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadConfig(dir)
	if err != nil || string(got) != "[hint]\n" {
		t.Errorf("ReadConfig = %q, %v", got, err)
	}
}

func TestReadConfigMissingMeansDefaults(t *testing.T) {
	for _, dir := range []string{"", t.TempDir()} {
		got, err := ReadConfig(dir)
		if err != nil || got != nil {
			t.Errorf("ReadConfig(%q) = %q, %v; want nil, nil", dir, got, err)
		}
	}
}

func TestReadConfigUnreadable(t *testing.T) {
	dir := t.TempDir()
	// A directory named config.toml cannot be read as a file.
	if err := os.Mkdir(filepath.Join(dir, "config.toml"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := ReadConfig(dir)
	if err == nil || !strings.HasPrefix(err.Error(), "config: ") {
		t.Errorf("ReadConfig = %v, want a config: error", err)
	}
}
