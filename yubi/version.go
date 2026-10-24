package main

import (
	_ "embed"
	"errors"
	"runtime/debug"

	"github.com/BurntSushi/toml"
)

//go:embed herdr-plugin.toml
var manifest []byte

// versionString returns "<version>+<commit>[.dirty]" from the manifest's
// version and the build's VCS stamp, or "<version>+unknown" without one.
func versionString(manifest []byte, info *debug.BuildInfo) (string, error) {
	var m struct {
		Version string `toml:"version"`
	}
	if _, err := toml.Decode(string(manifest), &m); err != nil {
		return "", err
	}
	if m.Version == "" {
		return "", errors.New("manifest has no version")
	}
	commit, dirty := "", false
	if info != nil {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				commit = s.Value
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
	}
	if commit == "" {
		return m.Version + "+unknown", nil
	}
	v := m.Version + "+" + commit[:min(7, len(commit))]
	if dirty {
		v += ".dirty"
	}
	return v, nil
}
