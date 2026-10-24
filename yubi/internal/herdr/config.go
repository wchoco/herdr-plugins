package herdr

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ReadConfig reads config.toml from yubi's plugin config dir, which herdr
// passes as HERDR_PLUGIN_CONFIG_DIR. An empty dir or a missing file means
// no config (nil, nil). There is no fallback path, and yubi never calls
// `herdr plugin config-dir`, which creates the dir as a side effect.
func ReadConfig(dir string) ([]byte, error) {
	if dir == "" {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return data, nil
}
