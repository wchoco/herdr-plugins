package herdr_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wchoco/herdr-plugins/yubi/internal/herdr"
)

// stub is a copy_command that saves its stdin to a file.
func stub(t *testing.T) (argv []string, out string) {
	t.Helper()
	out = filepath.Join(t.TempDir(), "clip")
	return []string{"sh", "-c", `cat > "$0"`, out}, out
}

func TestCopyCommandGetsTheTextOnStdinInsteadOfOSC52(t *testing.T) {
	argv, out := stub(t)
	var tty bytes.Buffer
	if err := herdr.NewClipboard(argv, &tty).Copy("emoji 🎉\nparty"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "emoji 🎉\nparty" {
		t.Errorf("stdin %q", got)
	}
	if tty.Len() != 0 {
		t.Errorf("wrote %q to the tty, want no OSC 52", tty.String())
	}
}

func TestNoCopyCommandMeansOSC52(t *testing.T) {
	var tty bytes.Buffer
	if err := herdr.NewClipboard(nil, &tty).Copy("hi"); err != nil {
		t.Fatal(err)
	}
	if got, want := tty.String(), "\x1b]52;c;aGk=\x07"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The 192 KiB limit is OSC 52's; a copy_command takes any size.
func TestCopyCommandHasNoSizeLimit(t *testing.T) {
	argv, out := stub(t)
	text := strings.Repeat("a", 200*1024)
	if err := herdr.NewClipboard(argv, &bytes.Buffer{}).Copy(text); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(out); len(got) != len(text) {
		t.Errorf("stdin got %d bytes, want %d", len(got), len(text))
	}
}

func TestCopyCommandErrors(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		text string
		want string
	}{
		{"non-zero exit with stderr", []string{"sh", "-c", "cat >/dev/null; echo 'no display' >&2; echo second >&2; exit 3"}, "x",
			"sh: exit status 3: no display"},
		{"non-zero exit, no stderr", []string{"false"}, "x", "false: exit status 1"},
		{"not found", []string{"yubi-no-such-copy-command"}, "x",
			`yubi-no-such-copy-command: exec: "yubi-no-such-copy-command": executable file not found in $PATH`},
		{"empty text", []string{"true"}, "", "nothing to copy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := herdr.NewClipboard(tt.argv, &bytes.Buffer{}).Copy(tt.text)
			if err == nil || err.Error() != tt.want {
				t.Errorf("got %v, want %q", err, tt.want)
			}
		})
	}
}

// Clipboard tools like xclip leave a background process holding the
// clipboard, with yubi's stderr pipe still open. Copy must return once
// the command itself exits.
func TestCopyCommandDoesNotWaitForBackgroundChildren(t *testing.T) {
	start := time.Now()
	err := herdr.NewClipboard([]string{"sh", "-c", "cat >/dev/null; sleep 10 & exit 0"}, &bytes.Buffer{}).Copy("x")
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("Copy took %v", d)
	}
}

func TestCopyCommandTimesOut(t *testing.T) {
	c := herdr.NewCommand([]string{"sleep", "10"})
	herdr.SetCommandTimeout(c, 100*time.Millisecond)
	start := time.Now()
	err := c.Copy("x")
	if err == nil || err.Error() != "sleep: timed out after 100ms" {
		t.Errorf("got %v", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("Copy took %v", d)
	}
}
