package session_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/config"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/session"
	"github.com/wchoco/herdr-plugins/yubi/internal/fixture"
)

// calls records Clipboard and Paster calls in the order they happen.
type calls struct {
	log      []string
	copyErr  error
	pasteErr error
}

type logClipboard struct{ c *calls }

func (l logClipboard) Copy(text string) error {
	l.c.log = append(l.c.log, "copy "+text)
	return l.c.copyErr
}

type logPaster struct{ c *calls }

func (l logPaster) Paste(_ context.Context, pane, text string) error {
	l.c.log = append(l.c.log, "paste "+pane+" "+text)
	return l.c.pasteErr
}

// runPaste runs a session on c with the config file toml (empty: defaults),
// recording copies and pastes in c's call log.
func runPaste(t *testing.T, mode session.Mode, scr fixture.Capture, toml string, c *calls, reads ...string) *fakeTerminal {
	t.Helper()
	cfg := session.Config{Mode: mode, Target: "w1:p1"}
	if toml != "" {
		settings, err := config.Parse([]byte(toml))
		if err != nil {
			t.Fatal(err)
		}
		cfg.Settings = &settings
	}
	term := newTerminal(120, 39, reads...)
	err := session.Run(context.Background(), cfg, session.Deps{
		Source:    &fakeSource{captures: map[string]fixture.Capture{"w1:p1": scr}},
		Clipboard: logClipboard{c},
		Paster:    logPaster{c},
		Terminal:  term,
	})
	if err != nil {
		t.Fatal(err)
	}
	return term
}

func TestLineModeAltEnterCopiesThenPastesTheLines(t *testing.T) {
	scr := screenOf("one   ", "two", "three")
	for name, altEnter := range map[string]string{"legacy": "\x1b\r", "kitty": "\x1b[13;3u"} {
		t.Run(name, func(t *testing.T) {
			c := &calls{}
			runPaste(t, session.ModeLine, scr, "", c, "3-2", altEnter)
			want := []string{"copy one\ntwo", "paste w1:p1 one\ntwo"}
			if !reflect.DeepEqual(c.log, want) {
				t.Errorf("got %q, want %q", c.log, want)
			}
		})
	}
}

func TestLineModeAltCtrlJPastesAJoinedCopy(t *testing.T) {
	// "one   " keeps its trailing spaces as an intermediate row.
	scr := screenOf("one   ", "two", "three")
	keys := map[string]string{
		"legacy alt-ctrl-j":    "\x1b\n",
		"kitty alt-ctrl-j":     "\x1b[106;7u",
		"kitty alt-ctrl-enter": "\x1b[13;7u",
	}
	for name, key := range keys {
		t.Run(name, func(t *testing.T) {
			c := &calls{}
			runPaste(t, session.ModeLine, scr, "", c, "3-2", key)
			want := []string{"copy one   two", "paste w1:p1 one   two"}
			if !reflect.DeepEqual(c.log, want) {
				t.Errorf("got %q, want %q", c.log, want)
			}
		})
	}
}

func TestLineModeAltOnAReboundCommitKeyPastes(t *testing.T) {
	scr := screenOf("one", "two")
	toml := "[line]\ncopy = [\"ctrl-y\"]\n"
	tests := []struct {
		name  string
		reads []string
		want  []string
	}{
		{name: "alt-ctrl-y pastes", reads: []string{"2", "\x1b[121;7u"}, want: []string{"copy one", "paste w1:p1 one"}},
		{name: "legacy alt-ctrl-y pastes", reads: []string{"2", "\x1b\x19"}, want: []string{"copy one", "paste w1:p1 one"}},
		{name: "ctrl-y copies only", reads: []string{"2", "\x19"}, want: []string{"copy one"}},
		{name: "alt-enter is no longer a commit key", reads: []string{"2", "\x1b[13;3u"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &calls{}
			runPaste(t, session.ModeLine, scr, toml, c, tt.reads...)
			if !reflect.DeepEqual(c.log, tt.want) {
				t.Errorf("got %q, want %q", c.log, tt.want)
			}
		})
	}
}

func TestPasteCopyCopyFailureShowsOnlyTheCopyErrorAndNoPaste(t *testing.T) {
	c := &calls{copyErr: errString("too long")}
	term := runPaste(t, session.ModeLine, screenOf("one"), "", c, "1", "\x1b\r")
	if want := []string{"copy one"}; !reflect.DeepEqual(c.log, want) {
		t.Errorf("got %q, want %q", c.log, want)
	}
	if got := statusRow(t, term); got != "copy failed: too long" {
		t.Errorf("status row %q", got)
	}
}

func TestPasteCopyPasteFailureIsReportedAndAnyKeyCloses(t *testing.T) {
	c := &calls{pasteErr: errString("pane_not_found")}
	// The key after the report closes the overlay; the last is never read.
	term := runPaste(t, session.ModeLine, screenOf("one"), "", c, "1", "\x1b\r", "x", "1")
	if want := []string{"copy one", "paste w1:p1 one"}; !reflect.DeepEqual(c.log, want) {
		t.Errorf("got %q, want %q", c.log, want)
	}
	if got := statusRow(t, term); got != "copied; paste failed: pane_not_found" {
		t.Errorf("status row %q", got)
	}
}

func TestPasteFailureReportOutlivesStaleEventsAndResizes(t *testing.T) {
	c := &calls{pasteErr: errString("pane_not_found")}
	l := &live{
		t:      t,
		src:    &fakeSource{captures: map[string]fixture.Capture{"w1:p1": fixture.Load(t, "cjk")}},
		clip:   &fakeClipboard{},
		term:   &fakeTerminal{w: 120, h: 39, input: make(chan []byte), resized: make(chan struct{})},
		events: &fakeEvents{ch: make(chan session.Event)},
		done:   make(chan error, 1),
	}
	deps := session.Deps{Source: l.src, Clipboard: l.clip, Paster: logPaster{c}, Terminal: l.term, Events: l.events}
	go func() { l.done <- session.Run(context.Background(), liveCfg, deps) }()

	l.key("3")
	l.key("\x1b\r")
	// Each of these would end the session silently; the report stays up.
	l.send(session.PaneClosed{Pane: "w1:p1"})
	l.send(session.PaneFocused{Pane: "w1:p1"})
	l.term.w = 100
	select {
	case l.term.resized <- struct{}{}:
	case <-time.After(time.Second):
		t.Fatal("session did not take the resize")
	}
	l.send(session.TabFocused{Tab: "w1:t2"})
	close(l.events.ch) // the end of the event stream too
	l.key("x")
	select {
	case err := <-l.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("session still running after a key")
	}
	if got := statusRow(t, l.term); got != "copied; paste failed: pane_not_found" {
		t.Errorf("status row %q", got)
	}
	if g := l.term.frames[len(l.term.frames)-1]; g.W != 100 {
		t.Errorf("last frame is %d wide, want the resized 100", g.W)
	}
}

func TestPasteTextStripsControlBytesButNewlineAndTab(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{name: "plain", in: "ls -la", want: "ls -la"},
		{name: "newline and tab kept", in: "a\tb\nc", want: "a\tb\nc"},
		{name: "escape sequence", in: "x\x1b[201~rm -rf /\x1b]52;c;eA==\x07y", want: "x[201~rm -rf /]52;c;eA==y"},
		{name: "carriage return", in: "a\r\nb", want: "a\nb"},
		{name: "every C0 byte but newline and tab", in: "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x0b\x0c\x0d\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f", want: ""},
		{name: "DEL", in: "a\x7fb", want: "ab"},
		{name: "C1 controls", in: "a\u009b31mb\u0085c", want: "a31mbc"},
		{name: "raw 8-bit CSI byte", in: "a\x9b31mb", want: "a31mb"},
		{name: "wide glyphs kept", in: "全角 🎉", want: "全角 🎉"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := session.PasteText(tt.in); got != tt.want {
				t.Errorf("PasteText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHintModeUppercaseFirstCharacterPastes(t *testing.T) {
	// p/01 has the two-letter Hint "nq"; the second character's case
	// does not matter.
	for _, typed := range [][]string{{"N", "q"}, {"N", "Q"}, {"\x1b[110;2u", "q"}} {
		c := &calls{}
		runPaste(t, session.ModeHint, manyPaths(), "", c, typed...)
		if want := []string{"copy p/01", "paste w1:p1 p/01"}; !reflect.DeepEqual(c.log, want) {
			t.Errorf("%q: got %q, want %q", typed, c.log, want)
		}
	}
}

func TestHintModeUppercaseSingleLetterHintPastes(t *testing.T) {
	c := &calls{}
	runPaste(t, session.ModeHint, fixture.Load(t, "links"), "", c, "S")
	want := []string{"copy https://example.com/docs", "paste w1:p1 https://example.com/docs"}
	if !reflect.DeepEqual(c.log, want) {
		t.Errorf("got %q, want %q", c.log, want)
	}
}

func TestHintModeLowercaseFirstCharacterOnlyCopies(t *testing.T) {
	// Only the first character's case decides Paste Copy.
	c := &calls{}
	runPaste(t, session.ModeHint, manyPaths(), "", c, "n", "Q")
	if want := []string{"copy p/01"}; !reflect.DeepEqual(c.log, want) {
		t.Errorf("got %q, want %q", c.log, want)
	}
}

func TestHintModePasteMarkerGolden(t *testing.T) {
	m := session.NewHint(screen.ParseANSI(manyPaths().ANSI), 20, 31)
	m.Key(keyOf('N'))
	golden(t, "hint-many-paste", m.Frame().Dump())
}

func TestHintModePasteMarker(t *testing.T) {
	c := &calls{}
	term := runPaste(t, session.ModeHint, manyPaths(), "", c, "N")
	if got := statusRow(t, term); got != "HINT  n  paste" {
		t.Errorf("status row %q", got)
	}
	// Backspace back to no input drops the marker; "a" then only copies.
	c = &calls{}
	term = runPaste(t, session.ModeHint, manyPaths(), "", c, "N", "\x7f")
	if got := statusRow(t, term); got != "HINT" {
		t.Errorf("status row after backspace %q", got)
	}
	c = &calls{}
	runPaste(t, session.ModeHint, manyPaths(), "", c, "N", "\x7f", "a")
	if want := []string{"copy p/30"}; !reflect.DeepEqual(c.log, want) {
		t.Errorf("got %q, want %q", c.log, want)
	}
}
