// Package fixture loads the real herdr captures under yubi/testdata for
// tests in any yubi package.
package fixture

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// HerdrVersion is the herdr version the captures were taken from. It
// matches min_herdr_version in yubi's manifest.
const HerdrVersion = "0.9.5"

// Names lists every captured fixture.
var Names = []string{"shell", "cjk", "wrap", "links", "joined"}

// Dir returns the absolute path of yubi/testdata.
func Dir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata")
}

// Capture is one `pane read --source visible` capture in both formats.
type Capture struct {
	Name string
	Text string // --format text
	ANSI string // --format ansi
}

// Load reads the capture called name.
func Load(t testing.TB, name string) Capture {
	t.Helper()
	read := func(ext string) string {
		b, err := os.ReadFile(filepath.Join(Dir(), "herdr-"+HerdrVersion, name+ext))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	return Capture{Name: name, Text: read(".txt"), ANSI: read(".ansi")}
}

// All loads every capture in Names.
func All(t testing.TB) []Capture {
	t.Helper()
	var cs []Capture
	for _, n := range Names {
		cs = append(cs, Load(t, n))
	}
	return cs
}
