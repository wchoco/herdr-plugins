package session_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
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
	paneErr  map[string]error // a read of this pane fails
	hang     map[string]bool  // a read of this pane blocks until its context is done

	mu    sync.Mutex // reads of other panes run in the background
	reads []string
}

func (s *fakeSource) ReadVisible(ctx context.Context, pane string, f session.Format) (string, error) {
	s.mu.Lock()
	s.reads = append(s.reads, pane)
	s.mu.Unlock()
	if s.hang[pane] {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if err := s.paneErr[pane]; err != nil {
		return "", err
	}
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
	w, h    int
	input   chan []byte
	resized chan struct{}
	frames  []*render.Grid
}

func newTerminal(w, h int, reads ...string) *fakeTerminal {
	t := &fakeTerminal{w: w, h: h, input: make(chan []byte, len(reads))}
	for _, r := range reads {
		t.input <- []byte(r)
	}
	close(t.input)
	return t
}

func (t *fakeTerminal) Size() (int, int, error)  { return t.w, t.h, nil }
func (t *fakeTerminal) Input() <-chan []byte     { return t.input }
func (t *fakeTerminal) Resized() <-chan struct{} { return t.resized }
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

func TestLineModeRangesInEitherOrderCopyTheSameRows(t *testing.T) {
	want := "日本語のテキスト: wide glyphs\nmixed ASCII 漢字 and かな, then a URL https://example.com/路径\ntruecolor 色 and 256-colour"
	for _, in := range []string{"6-4\r", "4-6\r", "4,6-5,4\r"} {
		s := run(t, session.ModeLine, "cjk", nil, in)
		if !reflect.DeepEqual(s.clip.copies, []string{want}) {
			t.Errorf("%q: copies %q", in, s.clip.copies)
		}
	}
}

func TestLineModeNumbersAndRangesCopyInScreenOrder(t *testing.T) {
	s := run(t, session.ModeLine, "cjk", nil, "1,7,3-2\r")
	want := "user@host:~/proj$ cat notes.txt\nemoji 🎉 party\n全角 reverse\nuser@host:~/proj$"
	if !reflect.DeepEqual(s.clip.copies, []string{want}) {
		t.Errorf("copies %q, want %q", s.clip.copies, want)
	}
}

func TestLineModeInvalidInputShowsErrorAndStaysOpen(t *testing.T) {
	tests := []struct {
		input, status string
	}{
		{"3-\r", "LINE  3-  invalid input"},
		{",,\r", "LINE  ,,  invalid input"},
		{"0\r", "LINE  0  invalid input"},
		{"8\r", "LINE  8  no line 8"},
		{"1,3-9\r", "LINE  1,3-9  no line 9"},
	}
	for _, tt := range tests {
		s := run(t, session.ModeLine, "cjk", nil, tt.input)
		if len(s.clip.copies) != 0 {
			t.Errorf("%q: copied %q", tt.input, s.clip.copies)
		}
		if got := statusRow(t, s.term); got != tt.status {
			t.Errorf("%q: status row %q, want %q", tt.input, got, tt.status)
		}
	}
}

func TestLineModeFixingInvalidInputThenCopies(t *testing.T) {
	// Backspace removes the dangling "-" of "3-" after the error.
	s := run(t, session.ModeLine, "cjk", nil, "3-\r", "\x7f", "\r")
	if want := []string{"emoji 🎉 party"}; !reflect.DeepEqual(s.clip.copies, want) {
		t.Errorf("copies %q, want %q", s.clip.copies, want)
	}
}

func TestLineModeEditingKeys(t *testing.T) {
	tests := []struct {
		name  string
		reads []string
		want  string // status row after the reads
	}{
		{name: "backspace deletes one character", reads: []string{"3-5", "\x7f"}, want: "LINE  3-"},
		{name: "backspace on empty input", reads: []string{"\x7f"}, want: "LINE"},
		{name: "ctrl-u clears the input", reads: []string{"3-5,1", "\x15"}, want: "LINE"},
		{name: "kitty ctrl-u", reads: []string{"3", "\x1b[117;5u"}, want: "LINE"},
		{name: "other keys are ignored", reads: []string{"3", "x", " ", "\x1b3", "\x1b[A", "\t", "-4"}, want: "LINE  3-4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := run(t, session.ModeLine, "cjk", nil, tt.reads...)
			if s.err != nil || len(s.clip.copies) != 0 {
				t.Errorf("err %v, copies %q", s.err, s.clip.copies)
			}
			if got := statusRow(t, s.term); got != tt.want {
				t.Errorf("status row %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLineModePreviewUpdatesOnEveryKeystroke(t *testing.T) {
	// cjk: Line 7 is row 0 ... Line 1 is row 6.
	s := run(t, session.ModeLine, "cjk", nil, "5", "-", "3", ",", "7", "\x7f", "9", "\x15")
	want := [][]int{
		nil,          // first frame
		{2},          // 5
		{2},          // 5-
		{2, 3, 4},    // 5-3
		{2, 3, 4},    // 5-3,
		{0, 2, 3, 4}, // 5-3,7
		{2, 3, 4},    // 5-3,
		{2, 3, 4},    // 5-3,9 (9 is not on screen)
		nil,          // cleared
	}
	if len(s.term.frames) != len(want) {
		t.Fatalf("got %d frames, want %d", len(s.term.frames), len(want))
	}
	for i, f := range s.term.frames {
		if got := selectedRows(f); !reflect.DeepEqual(got, want[i]) {
			t.Errorf("frame %d: selected rows %v, want %v", i, got, want[i])
		}
	}
}

// selectedRows returns the rows whose last cell is drawn as selected: the
// selection spans the full width.
func selectedRows(g *render.Grid) []int {
	var rows []int
	for y := 0; y < g.H; y++ {
		if g.Cell(y, g.W-1).Role == render.Selected {
			rows = append(rows, y)
		}
	}
	return rows
}

func TestLineModeIgnoredKeyDoesNotClose(t *testing.T) {
	s := run(t, session.ModeLine, "cjk", nil, "x", "1\r")
	if want := []string{"user@host:~/proj$"}; !reflect.DeepEqual(s.clip.copies, want) {
		t.Errorf("copies %q, want %q", s.clip.copies, want)
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

// manyPaths is a hand-written screen of 30 distinct paths, one per row, so
// the top Matches get two-letter Hints from the default alphabet.
func manyPaths() fixture.Capture {
	var rows []string
	for i := 1; i <= 30; i++ {
		rows = append(rows, fmt.Sprintf("file p/%02d", i))
	}
	s := strings.Join(rows, "\n")
	return fixture.Capture{Name: "many", Text: s + "\n", ANSI: strings.ReplaceAll(s, "\n", "\r\n")}
}

func runCapture(t *testing.T, mode session.Mode, c fixture.Capture, reads ...string) scenario {
	t.Helper()
	s := scenario{
		src:  &fakeSource{captures: map[string]fixture.Capture{"w1:p1": c}},
		clip: &fakeClipboard{},
		term: newTerminal(120, 39, reads...),
	}
	s.err = session.Run(context.Background(),
		session.Config{Mode: mode, Target: "w1:p1"},
		session.Deps{Source: s.src, Clipboard: s.clip, Terminal: s.term})
	return s
}

func TestHintModeTypingAHintCopiesItsMatchOnceAndExits(t *testing.T) {
	// 30 unique texts: p/01 at the top gets the last two-letter Hint, "nq".
	// "x" continues no Hint and is ignored; the read after the copy is
	// never seen because Run has exited.
	s := runCapture(t, session.ModeHint, manyPaths(), "n", "x", "q", "a")
	if s.err != nil {
		t.Fatal(s.err)
	}
	if want := []string{"p/01"}; !reflect.DeepEqual(s.clip.copies, want) {
		t.Errorf("copies: got %q, want %q", s.clip.copies, want)
	}
	if got := statusRow(t, s.term); got != "HINT  n" {
		t.Errorf("status row %q", got)
	}
}

func TestHintModeSharedHintCopiesDuplicatedURL(t *testing.T) {
	// The URL printed three times shares the second-shortest Hint: the
	// prompt path on the last row is nearer the bottom.
	s := run(t, session.ModeHint, "links", nil, "s")
	if want := []string{"https://example.com/docs"}; !reflect.DeepEqual(s.clip.copies, want) {
		t.Errorf("copies: got %q, want %q", s.clip.copies, want)
	}
}

func TestHintModeBackspaceAndCancel(t *testing.T) {
	// Backspace drops the typed "n", so "a" completes the bottom Hint.
	s := runCapture(t, session.ModeHint, manyPaths(), "n", "\x7f", "a")
	if want := []string{"p/30"}; !reflect.DeepEqual(s.clip.copies, want) {
		t.Errorf("copies: got %q, want %q", s.clip.copies, want)
	}
	for _, key := range []string{"\x1b", "\x03"} {
		s := runCapture(t, session.ModeHint, manyPaths(), "n", key, "q")
		if s.err != nil || len(s.clip.copies) != 0 {
			t.Errorf("%q: err %v, copies %q", key, s.err, s.clip.copies)
		}
	}
}

func TestHintModeNoMatches(t *testing.T) {
	c := fixture.Capture{Text: "hello\nworld\n", ANSI: "hello\r\nworld"}
	s := runCapture(t, session.ModeHint, c, "a", "s")
	if got := statusRow(t, s.term); got != "no matches" {
		t.Errorf("status row %q", got)
	}
	if len(s.clip.copies) != 0 || len(s.term.frames) != 1 {
		t.Errorf("copies %q, %d frames", s.clip.copies, len(s.term.frames))
	}
}

func TestHintModeBlankScreenHasNothingToCopy(t *testing.T) {
	s := runCapture(t, session.ModeHint, fixture.Capture{}, "a")
	if got := statusRow(t, s.term); got != "nothing to copy" {
		t.Errorf("status row %q", got)
	}
}

func TestHintRenderGoldens(t *testing.T) {
	tests := []struct {
		name  string
		c     fixture.Capture
		w, h  int
		typed string
	}{
		{name: "hint-links", c: fixture.Load(t, "links"), w: 80, h: 14},
		{name: "hint-many", c: manyPaths(), w: 20, h: 31},
		{name: "hint-many-typed", c: manyPaths(), w: 20, h: 31, typed: "n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := session.NewHint(screen.ParseANSI(tt.c.ANSI), tt.w, tt.h)
			for _, r := range tt.typed {
				m.Key(keyOf(r))
			}
			golden(t, tt.name, m.Frame().Dump())
		})
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
		{name: "line-cjk-preview-partial", capture: "cjk", w: 80, h: 12, typed: "5-"},
		{name: "line-cjk-preview", capture: "cjk", w: 80, h: 12, typed: "6-4,1"},
		{name: "line-cjk-error-malformed", capture: "cjk", w: 80, h: 12, typed: "6-4,\r"},
		{name: "line-cjk-error-out-of-range", capture: "cjk", w: 80, h: 12, typed: "2-9\r"},
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
