package main

import (
	"runtime/debug"
	"testing"
)

func TestVersionString(t *testing.T) {
	manifest := []byte("id = \"wchoco.yubi\"\nversion = \"0.3.0\"\n")
	info := func(settings ...debug.BuildSetting) *debug.BuildInfo {
		return &debug.BuildInfo{Settings: settings}
	}
	rev := debug.BuildSetting{Key: "vcs.revision", Value: "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678"}
	tests := []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{name: "clean", info: info(rev, debug.BuildSetting{Key: "vcs.modified", Value: "false"}), want: "0.3.0+a1b2c3d"},
		{name: "dirty", info: info(rev, debug.BuildSetting{Key: "vcs.modified", Value: "true"}), want: "0.3.0+a1b2c3d.dirty"},
		{name: "no vcs info", info: info(), want: "0.3.0+unknown"},
		{name: "no build info", info: nil, want: "0.3.0+unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := versionString(manifest, tt.info)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// The embedded manifest is yubi's own and carries its version.
func TestEmbeddedManifestVersion(t *testing.T) {
	got, err := versionString(manifest, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "0.1.0+unknown" {
		t.Errorf("got %q", got)
	}
}
