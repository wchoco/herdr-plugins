package session_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/layout"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/render"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/session"
	"github.com/wchoco/herdr-plugins/yubi/internal/fixture"
)

// Tabs of 80x20 with herdr's default ui config give a 77x18 overlay (see
// the layout package's tests for where those numbers come from).

func pane(id string, x, y, w, h int) layout.Pane {
	return layout.Pane{ID: id, Rect: layout.Rect{X: x, Y: y, Width: w, Height: h}}
}

func tab80x20(target string, panes ...layout.Pane) *layout.Tab {
	return &layout.Tab{Area: layout.Rect{Width: 80, Height: 20}, Target: target, Panes: panes}
}

// twoPanes: the Target on the left, a neighbour on the right.
var twoPanes = tab80x20("w1:p1", pane("w1:p1", 0, 0, 40, 20), pane("w1:p2", 40, 0, 40, 20))

// threePanes: a neighbour on the left, the Target top right, a neighbour
// below it.
var threePanes = tab80x20("w1:p2", pane("w1:p1", 0, 0, 40, 20), pane("w1:p2", 40, 0, 40, 10), pane("w1:p3", 40, 10, 40, 10))

func parse(t *testing.T, name string) screen.Screen {
	return screen.ParseANSI(fixture.Load(t, name).ANSI)
}

func TestLayoutRenderGoldens(t *testing.T) {
	tests := []struct {
		name       string
		mode       session.Mode
		target     string
		tab        *layout.Tab
		w, h       int
		neighbours map[string]string // pane id -> capture; a missing pane stays blank
		typed      string
	}{
		// The Target re-renders over itself on the left; its last row is
		// under the status row. The neighbour, Pane Label "n", has its
		// own Line Numbers drawn after the label (n1, n2, ...).
		{name: "line-split2", mode: session.ModeLine, target: "cjk", tab: twoPanes, w: 77, h: 18,
			neighbours: map[string]string{"w1:p2": "shell"}, typed: "3"},
		// Labelled input selects rows of the neighbour only.
		{name: "line-split2-neighbour-selected", mode: session.ModeLine, target: "cjk", tab: twoPanes, w: 77, h: 18,
			neighbours: map[string]string{"w1:p2": "shell"}, typed: "n2-4"},
		// The neighbour has Pane Label "n": its Hints are drawn with it
		// (na, ns, ...).
		{name: "hint-split2", mode: session.ModeHint, target: "links", tab: twoPanes, w: 77, h: 18,
			neighbours: map[string]string{"w1:p2": "shell"}},
		// Typing the Pane Label leaves only the neighbour's Hints, with
		// the label drawn as typed.
		{name: "hint-split2-label-typed", mode: session.ModeHint, target: "links", tab: twoPanes, w: 77, h: 18,
			neighbours: map[string]string{"w1:p2": "shell"}, typed: "n"},
		// The Target top right is clipped at its own grid: rows below row
		// 8 get no Hints, URLs past its right edge get right-aligned
		// Hints. The left neighbour is labelled "n"; w1:p3, labelled
		// "b", is not read yet, so it is blank.
		{name: "hint-split3", mode: session.ModeHint, target: "links", tab: threePanes, w: 77, h: 18,
			neighbours: map[string]string{"w1:p1": "shell"}},
		// Both neighbours are numbered after their labels: "n" on the
		// left, "b" below the Target, counted from its own last
		// non-blank displayed row. The unlabelled input selects in the
		// Target only.
		{name: "line-split3", mode: session.ModeLine, target: "links", tab: threePanes, w: 77, h: 18,
			neighbours: map[string]string{"w1:p1": "shell", "w1:p3": "cjk"}, typed: "2-3"},
		// A full-tab Target: the status row is over its last displayed
		// row, the rows below are clipped, and so are long rows.
		{name: "hint-fulltab-clipped", mode: session.ModeHint, target: "links", w: 40, h: 10,
			tab: &layout.Tab{Area: layout.Rect{Width: 43, Height: 12}, Target: "w1:p1", Panes: []layout.Pane{pane("w1:p1", 0, 0, 43, 12)}}},
		// Two Matches off-screen on the last displayed row (/tmp/... then
		// ~/proj/...): their Hints go leftward from the edge in Match
		// order, so /tmp's "s" is in the last cell and ~/proj's "a" left
		// of it.
		{name: "hint-fulltab-narrow", mode: session.ModeHint, target: "links", w: 12, h: 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scr := parse(t, tt.target)
			p := layout.Place(tt.tab, tt.w, tt.h)
			var m *session.Machine
			if tt.mode == session.ModeHint {
				m = session.NewHintAt(scr, p)
			} else {
				m = session.NewLineAt(scr, p)
			}
			for id, c := range tt.neighbours {
				if got := m.Neighbour(id, parse(t, c)); !reflect.DeepEqual(got, []session.Action{session.Redraw{}}) {
					t.Errorf("Neighbour(%s) = %v, want a redraw", id, got)
				}
			}
			for _, r := range tt.typed {
				m.Key(keyOf(r))
			}
			golden(t, tt.name, m.Frame().Dump())
		})
	}
}

func TestNeighbourNotInLayoutIsIgnored(t *testing.T) {
	m := session.NewLineAt(parse(t, "cjk"), layout.Place(twoPanes, 77, 18))
	if got := m.Neighbour("w1:p1", parse(t, "shell")); got != nil {
		t.Errorf("Target as neighbour: %v", got)
	}
	if got := m.Neighbour("w1:p9", parse(t, "shell")); got != nil {
		t.Errorf("unknown pane: %v", got)
	}
}

func TestUnreadableNeighbourStaysBlankWithNoMessage(t *testing.T) {
	for _, mode := range []session.Mode{session.ModeLine, session.ModeHint} {
		t.Run(string(mode), func(t *testing.T) {
			p := layout.Place(twoPanes, 77, 18)
			m := session.NewLineAt(parse(t, "links"), p)
			if mode == session.ModeHint {
				m = session.NewHintAt(parse(t, "links"), p)
			}
			before := m.Frame().Dump()
			if got := m.NeighbourUnreadable("w1:p2", errors.New("pane_not_found")); got != nil {
				t.Errorf("NeighbourUnreadable = %v, want nothing to do", got)
			}
			if got := m.Frame().Dump(); got != before {
				t.Errorf("frame changed:\n%s\nwant\n%s", got, before)
			}
		})
	}
}

func TestHintModeRightClippedMatchCopiesItsFullText(t *testing.T) {
	// In a 40-wide overlay ~/proj/out.txt on row 8 starts at column 39,
	// past its Hint's room; its Hint "a" sits at the right edge (golden
	// hint-fulltab-clipped).
	s := scenario{
		src:  &fakeSource{captures: map[string]fixture.Capture{"w1:p1": fixture.Load(t, "links")}},
		clip: &fakeClipboard{},
		term: newTerminal(40, 10, "a"),
	}
	s.err = session.Run(context.Background(),
		session.Config{Mode: session.ModeHint, Target: "w1:p1"},
		session.Deps{Source: s.src, Clipboard: s.clip, Terminal: s.term})
	if want := []string{"~/proj/out.txt"}; !reflect.DeepEqual(s.clip.copies, want) {
		t.Errorf("copies: got %q, want %q", s.clip.copies, want)
	}
}

func TestLineModeRowsClippedAtTheBottomCannotBeSelected(t *testing.T) {
	// links has 12 rows; the Target top right of threePanes shows 8, so
	// the top displayed row is Line 8 and 9 is out of range.
	m := session.NewLineAt(parse(t, "links"), layout.Place(threePanes, 77, 18))
	for _, r := range "9\r" {
		m.Key(keyOf(r))
	}
	if got := strings.TrimRight(m.Frame().RowText(17), " "); !strings.Contains(got, "no line 9") {
		t.Errorf("status row %q", got)
	}
	m = session.NewLineAt(parse(t, "links"), layout.Place(threePanes, 77, 18))
	var copied []session.Action
	for _, r := range "8\r" {
		copied = m.Key(keyOf(r))
	}
	if want := []session.Action{session.Copy{Text: "user@host:~/proj$ git push"}}; !reflect.DeepEqual(copied, want) {
		t.Errorf("got %v, want %v", copied, want)
	}
}

// liveTerminal is a Terminal whose input stays open until the test closes
// it, and which hands every frame to the test.
type liveTerminal struct {
	w, h   int
	input  chan []byte
	frames chan *render.Grid
}

func newLiveTerminal(w, h int) *liveTerminal {
	return &liveTerminal{w: w, h: h, input: make(chan []byte, 8), frames: make(chan *render.Grid, 64)}
}

func (t *liveTerminal) Size() (int, int, error)  { return t.w, t.h, nil }
func (t *liveTerminal) Input() <-chan []byte     { return t.input }
func (t *liveTerminal) Resized() <-chan struct{} { return nil }
func (t *liveTerminal) Draw(g *render.Grid) error {
	t.frames <- g
	return nil
}

func (t *liveTerminal) next(tb testing.TB) *render.Grid {
	tb.Helper()
	select {
	case g := <-t.frames:
		return g
	case <-time.After(5 * time.Second):
		tb.Fatal("no frame drawn")
		return nil
	}
}

func runLive(t *testing.T, src *fakeSource, clip *fakeClipboard, term *liveTerminal, tab *layout.Tab, mode session.Mode) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- session.Run(context.Background(),
			session.Config{Mode: mode, Target: tab.Target, Layout: tab},
			session.Deps{Source: src, Clipboard: clip, Terminal: term})
	}()
	return done
}

func wait(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestSlowNeighbourNeverDelaysTheOverlay(t *testing.T) {
	src := &fakeSource{
		captures: map[string]fixture.Capture{"w1:p1": fixture.Load(t, "cjk")},
		hang:     map[string]bool{"w1:p2": true},
	}
	clip := &fakeClipboard{}
	term := newLiveTerminal(77, 18)
	done := runLive(t, src, clip, term, twoPanes, session.ModeLine)
	// The first frame already shows the Target, with the neighbour blank.
	first := term.next(t)
	if got := first.RowText(0); !strings.Contains(got, "ser@host") {
		t.Errorf("first frame row 0 %q", got)
	}
	if got := strings.TrimSpace(first.RowText(0)[strings.Index(first.RowText(0), "│")+len("│"):]); got != "│" {
		t.Errorf("neighbour not blank: %q", first.RowText(0))
	}
	term.input <- []byte("1\r")
	wait(t, done)
	if want := []string{"user@host:~/proj$"}; !reflect.DeepEqual(clip.copies, want) {
		t.Errorf("copies %q, want %q", clip.copies, want)
	}
}

func TestNeighbourIsNumberedWhenItsReadArrives(t *testing.T) {
	src := &fakeSource{captures: map[string]fixture.Capture{"w1:p1": fixture.Load(t, "cjk"), "w1:p2": fixture.Load(t, "shell")}}
	term := newLiveTerminal(77, 18)
	done := runLive(t, src, &fakeClipboard{}, term, twoPanes, session.ModeLine)
	var g *render.Grid
	for g = term.next(t); !strings.Contains(g.RowText(0), "git status"); g = term.next(t) {
	}
	// The neighbour's grid starts at column 40 of the overlay. Its rows
	// are drawn like the Target's, Line Numbers after its Pane Label "n"
	// over the first cells ("user..." under "n16").
	if c := g.Cell(0, 40); c.Text != "n" || c.Role != render.LineNumber {
		t.Errorf("neighbour label cell %+v", c)
	}
	if c := g.Cell(0, 43); c.Text != "r" || c.Role != render.Text {
		t.Errorf("neighbour text cell %+v", c)
	}
	close(term.input)
	wait(t, done)
	src.mu.Lock()
	defer src.mu.Unlock()
	if want := []string{"w1:p1", "w1:p2"}; !sameSet(src.reads, want) {
		t.Errorf("reads %q, want %q once each", src.reads, want)
	}
}

func TestFailingNeighbourLeavesItsRectBlankSilently(t *testing.T) {
	src := &fakeSource{
		captures: map[string]fixture.Capture{"w1:p1": fixture.Load(t, "cjk")},
		paneErr:  map[string]error{"w1:p2": errors.New("pane_not_found")},
	}
	clip := &fakeClipboard{}
	term := newLiveTerminal(77, 18)
	done := runLive(t, src, clip, term, twoPanes, session.ModeLine)
	term.next(t)
	term.input <- []byte("1\r")
	wait(t, done)
	if want := []string{"user@host:~/proj$"}; !reflect.DeepEqual(clip.copies, want) {
		t.Errorf("copies %q, want %q", clip.copies, want)
	}
}

func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]int{}
	for _, s := range got {
		seen[s]++
	}
	for _, s := range want {
		if seen[s] != 1 {
			return false
		}
	}
	return true
}
