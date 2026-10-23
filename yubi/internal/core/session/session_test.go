package session_test

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/keys"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/render"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/session"
	"github.com/wchoco/herdr-plugins/yubi/internal/fixture"
)

var update = flag.Bool("update", false, "rewrite render goldens")

// Fakes for the adapter interfaces.

type fakeSource struct {
	captures map[string]fixture.Capture // by pane id
	err      error
	reads    []string
}

func (s *fakeSource) ReadVisible(_ context.Context, pane string, f session.Format) (string, error) {
	s.reads = append(s.reads, pane)
	if s.err != nil {
		return "", s.err
	}
	c := s.captures[pane]
	if f == session.FormatANSI {
		return c.ANSI, nil
	}
	return c.Text, nil
}

type fakeClipboard struct {
	copies []string
	err    error
}

func (c *fakeClipboard) Copy(text string) error {
	c.copies = append(c.copies, text)
	return c.err
}

type fakeTerminal struct {
	w, h   int
	input  chan []byte
	frames []*render.Grid
}

func newTerminal(w, h int, reads ...string) *fakeTerminal {
	t := &fakeTerminal{w: w, h: h, input: make(chan []byte, len(reads))}
	for _, r := range reads {
		t.input <- []byte(r)
	}
	close(t.input)
	return t
}

func (t *fakeTerminal) Size() (int, int, error) { return t.w, t.h, nil }
func (t *fakeTerminal) Input() <-chan []byte    { return t.input }
func (t *fakeTerminal) Draw(frame *render.Grid) error {
	t.frames = append(t.frames, frame)
	return nil
}

type scenario struct {
	src  *fakeSource
	clip *fakeClipboard
	term *fakeTerminal
	err  error
}

func run(t *testing.T, mode session.Mode, capture string, clipErr error, reads ...string) scenario {
	t.Helper()
	s := scenario{
		src:  &fakeSource{captures: map[string]fixture.Capture{"w1:p1": fixture.Load(t, capture)}},
		clip: &fakeClipboard{err: clipErr},
		term: newTerminal(120, 39, reads...),
	}
	s.err = session.Run(context.Background(),
		session.Config{Mode: mode, Target: "w1:p1"},
		session.Deps{Source: s.src, Clipboard: s.clip, Terminal: s.term})
	return s
}

func TestLineModeCopiesTypedLineAndExits(t *testing.T) {
	// The last read would be ignored: Run has exited after Enter.
	s := run(t, session.ModeLine, "cjk", nil, "3", "\r", "4\r")
	if s.err != nil {
		t.Fatal(s.err)
	}
	if want := []string{"emoji 🎉 party"}; !reflect.DeepEqual(s.clip.copies, want) {
		t.Errorf("copies: got %q, want %q", s.clip.copies, want)
	}
	if want := []string{"w1:p1"}; !reflect.DeepEqual(s.src.reads, want) {
		t.Errorf("reads: got %q, want %q", s.src.reads, want)
	}
	// First frame, then one after typing 3.
	if len(s.term.frames) != 2 {
		t.Errorf("got %d frames, want 2", len(s.term.frames))
	}
}

func TestLineModeMultiDigitNumberInOneRead(t *testing.T) {
	s := run(t, session.ModeLine, "shell", nil, "15\r")
	if want := []string{"        modified:   yubi/main.go"}; !reflect.DeepEqual(s.clip.copies, want) {
		t.Errorf("copies: got %q, want %q", s.clip.copies, want)
	}
}

func TestLineModeEnterOnEmptyInputDoesNothing(t *testing.T) {
	s := run(t, session.ModeLine, "cjk", nil, "\r", "1\r")
	if want := []string{"user@host:~/proj$"}; !reflect.DeepEqual(s.clip.copies, want) {
		t.Errorf("copies: got %q, want %q", s.clip.copies, want)
	}
}

func TestLineModeNumberAboveTopRowCopiesNothing(t *testing.T) {
	s := run(t, session.ModeLine, "cjk", nil, "8\r")
	if len(s.clip.copies) != 0 {
		t.Errorf("copied %q", s.clip.copies)
	}
}

func TestCancelKeysExitWithoutCopy(t *testing.T) {
	for _, key := range []string{"\x03", "\x1b[27u", "\x1b[99;5u"} {
		s := run(t, session.ModeLine, "cjk", nil, "3", key, "\r")
		if s.err != nil || len(s.clip.copies) != 0 {
			t.Errorf("%q: err %v, copies %q", key, s.err, s.clip.copies)
		}
	}
}

func TestCopyFailureIsShownAndAnyKeyCloses(t *testing.T) {
	s := run(t, session.ModeLine, "cjk", errors.New("text too large"), "1\r", "x", "2\r")
	if len(s.clip.copies) != 1 {
		t.Errorf("got %d copies, want 1", len(s.clip.copies))
	}
	if got := statusRow(t, s.term); got != "copy failed: text too large" {
		t.Errorf("status row %q", got)
	}
}

func TestReadFailureIsShownAndAnyKeyCloses(t *testing.T) {
	src := &fakeSource{err: errors.New("pane_not_found")}
	clip := &fakeClipboard{}
	term := newTerminal(80, 10, "1\r")
	err := session.Run(context.Background(),
		session.Config{Mode: session.ModeLine, Target: "w1:p9"},
		session.Deps{Source: src, Clipboard: clip, Terminal: term})
	if err != nil {
		t.Fatal(err)
	}
	if got := statusRow(t, term); got != "cannot read target: pane_not_found" {
		t.Errorf("status row %q", got)
	}
	if len(clip.copies) != 0 || len(term.frames) != 1 {
		t.Errorf("copies %q, %d frames", clip.copies, len(term.frames))
	}
}

func TestRenderGoldens(t *testing.T) {
	tests := []struct {
		name    string
		capture string
		w, h    int
		typed   string
	}{
		{name: "line-cjk", capture: "cjk", w: 80, h: 12, typed: "3"},
		{name: "line-shell-clipped", capture: "shell", w: 60, h: 12},
		{name: "line-wrap", capture: "wrap", w: 120, h: 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scr := screen.ParseANSI(fixture.Load(t, tt.capture).ANSI)
			m := session.NewLine(scr, tt.w, tt.h)
			for _, r := range tt.typed {
				m.Key(keyOf(r))
			}
			golden(t, tt.name, m.Frame().Dump())
		})
	}
}

func keyOf(r rune) keys.Key { return keys.Key{Code: r} }

// statusRow returns the text of the bottom row of the last frame drawn.
func statusRow(t *testing.T, term *fakeTerminal) string {
	t.Helper()
	if len(term.frames) == 0 {
		t.Fatal("no frame drawn")
	}
	g := term.frames[len(term.frames)-1]
	return strings.TrimRight(g.RowText(g.H-1), " ")
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join(fixture.Dir(), "golden", name+".txt")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("frame differs from %s (run go test -update after checking):\n%s", path, got)
	}
}
