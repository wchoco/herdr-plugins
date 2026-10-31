package session_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/config"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/layout"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/render"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/session"
	"github.com/wchoco/herdr-plugins/yubi/internal/fixture"
)

// splitRun is a session running live on a split tab, with copies and
// pastes recorded in its call log.
type splitRun struct {
	src  *fakeSource
	c    *calls
	term *liveTerminal
	done <-chan error
}

// startSplit runs a session in mode on tab, reading each pane from
// captures, with the config file toml (empty: defaults). A pane in gated
// is not read until the test calls release.
func startSplit(t *testing.T, mode session.Mode, tab *layout.Tab, toml string, captures map[string]fixture.Capture, gated ...string) *splitRun {
	t.Helper()
	return startSplitWith(t, mode, tab, toml, captures, nil, nil, gated...)
}

// startSplitWith is startSplit where a read of a pane in failing fails
// with its error, and the session is watched through events when it is
// not nil.
func startSplitWith(t *testing.T, mode session.Mode, tab *layout.Tab, toml string, captures map[string]fixture.Capture, failing map[string]error, events *fakeEvents, gated ...string) *splitRun {
	t.Helper()
	cfg := session.Config{Mode: mode, Target: tab.Target, Layout: tab}
	if toml != "" {
		settings, err := config.Parse([]byte(toml))
		if err != nil {
			t.Fatal(err)
		}
		cfg.Settings = &settings
	}
	r := &splitRun{
		src:  &fakeSource{captures: captures, paneErr: failing, gate: map[string]chan struct{}{}},
		c:    &calls{},
		term: newLiveTerminal(77, 18),
	}
	for _, id := range gated {
		r.src.gate[id] = make(chan struct{})
	}
	d := session.Deps{Source: r.src, Clipboard: logClipboard{r.c}, Paster: logPaster{r.c}, Terminal: r.term}
	if events != nil {
		d.Events = events
	}
	done := make(chan error, 1)
	go func() { done <- session.Run(context.Background(), cfg, d) }()
	r.done = done
	return r
}

// arrived waits for the first frame and then for one frame per neighbour
// read that delivers, and returns the last frame.
func (r *splitRun) arrived(t *testing.T, reads int) *render.Grid {
	t.Helper()
	g := r.term.next(t)
	for range reads {
		g = r.term.next(t)
	}
	return g
}

// release lets the gated read of pane return, and returns the frame drawn
// when it arrives.
func (r *splitRun) release(t *testing.T, pane string) *render.Grid {
	t.Helper()
	close(r.src.gate[pane])
	return r.term.next(t)
}

// press types s, which must redraw, and returns the frame drawn.
func (r *splitRun) press(t *testing.T, s string) *render.Grid {
	t.Helper()
	r.term.input <- []byte(s)
	return r.term.next(t)
}

// finish types s and waits for the session to end, and returns the
// copies and pastes made.
func (r *splitRun) finish(t *testing.T, s string) []string {
	t.Helper()
	r.term.input <- []byte(s)
	wait(t, r.done)
	return r.c.log
}

// pathsScreen is a screen of n distinct paths /p1 /p2 ..., six to a row.
func pathsScreen(n int) fixture.Capture {
	var rows []string
	row := ""
	for i := 1; i <= n; i++ {
		row += fmt.Sprintf("/p%d ", i)
		if i%6 == 0 || i == n {
			rows = append(rows, row)
			row = ""
		}
	}
	return screenOf(rows...)
}

// rightTall: the Target top left, a neighbour below it, and a tall
// neighbour on the right. Tab order lists the lower left pane first, but
// in reading order the right pane comes first: its top is higher.
var rightTall = tab80x20("w1:p1", pane("w1:p1", 0, 0, 40, 10), pane("w1:p2", 0, 10, 40, 10), pane("w1:p3", 40, 0, 40, 20))

func TestPaneLabelsComeFromTheAlphabetsEndInReadingOrder(t *testing.T) {
	// 27 Matches on the Target: with all 26 letters its Hints would use
	// "n" as a prefix, but the Target never gets the label letters.
	captures := map[string]fixture.Capture{
		"w1:p1": pathsScreen(27),
		"w1:p2": screenOf("lower /left"),
		"w1:p3": screenOf("tall /right"),
	}
	for typed, want := range map[string]string{"na": "copy /right", "ba": "copy /left"} {
		r := startSplit(t, session.ModeHint, rightTall, "", captures)
		r.arrived(t, 2)
		if got := r.finish(t, typed); !reflect.DeepEqual(got, []string{want}) {
			t.Errorf("%s: got %q, want %q", typed, got, want)
		}
	}
}

// drawnHints returns the Hints drawn in g in columns [x0, x1), typed
// prefix included, in reading order.
func drawnHints(g *render.Grid, x0, x1 int) []string {
	var hs []string
	for y := 0; y < g.H-1; y++ {
		h := ""
		for x := x0; x <= x1; x++ {
			if x < x1 {
				if c := g.Cell(y, x); c.Role == render.Hint || c.Role == render.HintTyped {
					h += c.Text
					continue
				}
			}
			if h != "" {
				hs = append(hs, h)
				h = ""
			}
		}
	}
	return hs
}

// In twoPanes the Target's grid is left of column 39 and the neighbour's
// right of it.
const splitCol = 39

func TestPaneLabelTypedBeforeItsPaneArrivesStaysTyped(t *testing.T) {
	r := startSplit(t, session.ModeHint, twoPanes, "", map[string]fixture.Capture{
		"w1:p1": screenOf("target /one"),
		"w1:p2": screenOf("neighbour /two"),
	}, "w1:p2")
	r.arrived(t, 0)
	g := r.press(t, "n")
	if got := statusText(g); got != "HINT  n" {
		t.Errorf("status %q after typing the label", got)
	}
	if got := drawnHints(g, 0, g.W); got != nil {
		t.Errorf("Hints %q still drawn after typing the label", got)
	}
	g = r.release(t, "w1:p2")
	if got, want := drawnHints(g, splitCol, g.W), []string{"na"}; !reflect.DeepEqual(got, want) {
		t.Errorf("neighbour Hints %q, want %q", got, want)
	}
	if got := drawnHints(g, 0, splitCol); got != nil {
		t.Errorf("Target Hints %q drawn under a typed label", got)
	}
	if got, want := r.finish(t, "a"), []string{"copy /two"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// statusText returns the text of g's status row.
func statusText(g *render.Grid) string { return strings.TrimRight(g.RowText(g.H-1), " ") }

func TestNeighbourHintCopiesThatPanesMatch(t *testing.T) {
	r := startSplit(t, session.ModeHint, twoPanes, "", map[string]fixture.Capture{
		"w1:p1": screenOf("target /one"),
		"w1:p2": screenOf("neighbour /two"),
	})
	r.arrived(t, 1)
	if got, want := r.finish(t, "na"), []string{"copy /two"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSameTextGetsOneHintInEachPane(t *testing.T) {
	r := startSplit(t, session.ModeHint, twoPanes, "", map[string]fixture.Capture{
		"w1:p1": screenOf("/same /same"),
		"w1:p2": screenOf("/same"),
	})
	g := r.arrived(t, 1)
	if got, want := drawnHints(g, 0, splitCol), []string{"a", "a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Target Hints %q, want %q", got, want)
	}
	if got, want := drawnHints(g, splitCol, g.W), []string{"na"}; !reflect.DeepEqual(got, want) {
		t.Errorf("neighbour Hints %q, want %q", got, want)
	}
	close(r.term.input)
	wait(t, r.done)
}

func TestHintsOnScreenDoNotChangeWhenASlowNeighbourArrives(t *testing.T) {
	r := startSplit(t, session.ModeHint, twoPanes, "", map[string]fixture.Capture{
		"w1:p1": pathsScreen(27),
		"w1:p2": pathsScreen(27),
	}, "w1:p2")
	before := r.arrived(t, 0)
	after := r.release(t, "w1:p2")
	if got := drawnHints(after, splitCol, after.W); len(got) != 27 {
		t.Errorf("neighbour got %d Hints, want 27: %q", len(got), got)
	}
	for y := 0; y < before.H-1; y++ {
		for x := 0; x < splitCol; x++ {
			if b, a := before.Cell(y, x), after.Cell(y, x); b != a {
				t.Fatalf("Target cell (%d,%d) changed from %+v to %+v", y, x, b, a)
			}
		}
	}
	close(r.term.input)
	wait(t, r.done)
}

func TestUppercaseNeighbourHintPastesIntoTheTargetPane(t *testing.T) {
	r := startSplit(t, session.ModeHint, twoPanes, "", map[string]fixture.Capture{
		"w1:p1": screenOf("target /one"),
		"w1:p2": screenOf("neighbour /two"),
	})
	r.arrived(t, 1)
	if got, want := r.finish(t, "Na"), []string{"copy /two", "paste w1:p1 /two"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestShortAlphabetKeepsTwoLettersForTheTarget(t *testing.T) {
	// "jkl" leaves one label, "l", for the first neighbour in reading
	// order (the tall right one); the lower left neighbour gets none and
	// is only drawn faint.
	captures := map[string]fixture.Capture{
		"w1:p1": screenOf("/one /two /three"),
		"w1:p2": screenOf("lower /left"),
		"w1:p3": screenOf("tall /right"),
	}
	r := startSplit(t, session.ModeHint, rightTall, `hint.alphabet = "jkl"`, captures)
	g := r.arrived(t, 2)
	// The Target's Hints come from "jk", bottom-right first; the right
	// neighbour's from all of "jkl" after its label.
	if got, want := drawnHints(g, 0, g.W), []string{"kk", "kj", "j", "lj"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Hints %q, want %q", got, want)
	}
	found := false
	for y := 0; y < g.H-1; y++ {
		found = found || strings.Contains(g.RowText(y), "lower /left")
	}
	if !found {
		t.Error("unlabelled neighbour not drawn")
	}
	if got, want := r.finish(t, "lj"), []string{"copy /right"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestZoomedTabGivesTheTargetTheWholeAlphabet(t *testing.T) {
	zoomed := *twoPanes
	zoomed.Zoomed = true
	// 27 Matches: the last letter, "n", becomes a prefix: /p2 gets "na".
	r := startSplit(t, session.ModeHint, &zoomed, "", map[string]fixture.Capture{
		"w1:p1": pathsScreen(27),
		"w1:p2": screenOf("neighbour /two"),
	})
	r.arrived(t, 0)
	if got, want := r.finish(t, "na"), []string{"copy /p2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	r.src.mu.Lock()
	defer r.src.mu.Unlock()
	if want := []string{"w1:p1"}; !reflect.DeepEqual(r.src.reads, want) {
		t.Errorf("reads %q, want %q", r.src.reads, want)
	}
}

// fiveRows is a screen whose Line Numbers 1 to 5 are "five" up to "one".
var fiveRows = screenOf("one", "two", "three", "four", "five")

func TestLabelledLineInputCopiesThatPanesRows(t *testing.T) {
	r := startSplit(t, session.ModeLine, twoPanes, "", map[string]fixture.Capture{
		"w1:p1": screenOf("target top", "target bottom"),
		"w1:p2": fiveRows,
	})
	r.arrived(t, 1)
	if got, want := r.finish(t, "n2-4\r"), []string{"copy two\nthree\nfour"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUnlabelledLineInputStillMeansTheTargetPane(t *testing.T) {
	r := startSplit(t, session.ModeLine, twoPanes, "", map[string]fixture.Capture{
		"w1:p1": screenOf("target top", "target bottom"),
		"w1:p2": fiveRows,
	})
	r.arrived(t, 1)
	if got, want := r.finish(t, "2,1\r"), []string{"copy target top\ntarget bottom"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBadLabelledLineInputShowsWhyAndCopiesNothing(t *testing.T) {
	// "jkl" labels only the first neighbour in reading order ("l", the
	// tall right pane); the lower left one has no label to type.
	tests := []struct {
		name, toml, input, want string
	}{
		{name: "mixed panes", input: "3,n2", want: "LINE  3,n2  one pane at a time"},
		{name: "mixed labels", input: "n2,b1", want: "LINE  n2,b1  one pane at a time"},
		{name: "no such label", input: "x1", want: "LINE  x1  invalid input"},
		{name: "no such label after the Target", input: "3,x1", want: "LINE  3,x1  invalid input"},
		{name: "no such label after a pane", input: "n2,x1", want: "LINE  n2,x1  invalid input"},
		{name: "unlabelled pane after the Target", toml: `hint.alphabet = "jkl"`, input: "3,k1", want: "LINE  3,k1  invalid input"},
		{name: "label alone", input: "n", want: "LINE  n  invalid input"},
		{name: "unlabelled pane", toml: `hint.alphabet = "jkl"`, input: "k1", want: "LINE  k1  invalid input"},
		{name: "no such line", input: "n9", want: "LINE  n9  no line 9"},
		{name: "blank rows", input: "b2", want: "LINE  b2  nothing to copy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := startSplit(t, session.ModeLine, rightTall, tt.toml, map[string]fixture.Capture{
				"w1:p1": fiveRows,
				"w1:p2": screenOf("lower", "", "", "left"),
				"w1:p3": fiveRows,
			})
			r.arrived(t, 2)
			for _, c := range tt.input {
				r.press(t, string(c))
			}
			if got := statusText(r.press(t, "\r")); got != tt.want {
				t.Errorf("status %q, want %q", got, tt.want)
			}
			if got := r.finish(t, "\x03"); got != nil {
				t.Errorf("got %q, want nothing copied", got)
			}
		})
	}
}

func TestLineCommitOnANeighbourNotReadYetWaitsForIt(t *testing.T) {
	r := startSplit(t, session.ModeLine, twoPanes, "", map[string]fixture.Capture{
		"w1:p1": screenOf("target"),
		"w1:p2": fiveRows,
	}, "w1:p2")
	r.arrived(t, 0)
	r.press(t, "n")
	r.press(t, "1")
	if got, want := statusText(r.press(t, "\r")), "LINE  n1  pane n not read yet"; got != want {
		t.Errorf("status %q, want %q", got, want)
	}
	r.release(t, "w1:p2")
	if got, want := r.finish(t, "\r"), []string{"copy five"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNeighbourLinesJoinedAndPasted(t *testing.T) {
	tests := []struct {
		name, toml, keys string
		want             []string
	}{
		{name: "joined copy", toml: `line.separator = " "`, keys: "n3-2\x0a", want: []string{"copy three four"}},
		{name: "paste copy", keys: "n1\x1b\r", want: []string{"copy five", "paste w1:p1 five"}},
		{name: "joined paste copy", toml: `line.separator = "+"`, keys: "n2-1\x1b\x0a", want: []string{"copy four+five", "paste w1:p1 four+five"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := startSplit(t, session.ModeLine, twoPanes, tt.toml, map[string]fixture.Capture{
				"w1:p1": screenOf("target"),
				"w1:p2": fiveRows,
			})
			r.arrived(t, 1)
			if got := r.finish(t, tt.keys); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPaneLabelsNameTheSamePaneInBothModes(t *testing.T) {
	// rightTall: "n" is the tall right pane and "b" the lower left one,
	// in Hint Mode (see above) and in Line Mode alike.
	captures := map[string]fixture.Capture{
		"w1:p1": screenOf("target"),
		"w1:p2": screenOf("lower left"),
		"w1:p3": screenOf("tall right"),
	}
	for input, want := range map[string]string{"n1\r": "copy tall right", "b1\r": "copy lower left"} {
		r := startSplit(t, session.ModeLine, rightTall, "", captures)
		r.arrived(t, 2)
		if got := r.finish(t, input); !reflect.DeepEqual(got, []string{want}) {
			t.Errorf("%q: got %q, want %q", input, got, want)
		}
	}
}

func TestNeighbourSelectionIsDrawnInThatPaneOnly(t *testing.T) {
	r := startSplit(t, session.ModeLine, twoPanes, "", map[string]fixture.Capture{
		"w1:p1": fiveRows,
		"w1:p2": fiveRows,
	})
	r.arrived(t, 1)
	r.press(t, "n")
	g := r.press(t, "2")
	// Line 2 is row 3 in both panes; only the neighbour's is selected.
	if c := g.Cell(3, splitCol+5); c.Role != render.Selected {
		t.Errorf("neighbour row 3 cell %+v, want selected", c)
	}
	if c := g.Cell(3, 4); c.Role == render.Selected {
		t.Errorf("Target row 3 cell %+v selected", c)
	}
	close(r.term.input)
	wait(t, r.done)
}

func TestUnreadableNeighbourKeepsItsLabelAndIsNoted(t *testing.T) {
	// rightTall: the tall right pane "n" fails; the lower left pane keeps
	// "b", the label it gets on every other launch.
	r := startSplitWith(t, session.ModeHint, rightTall, "", map[string]fixture.Capture{
		"w1:p1": screenOf("target /one"),
		"w1:p2": screenOf("lower /left"),
	}, map[string]error{"w1:p3": errors.New("pane_not_found")}, nil)
	g := r.arrived(t, 2)
	if got := statusText(g); !strings.HasSuffix(got, "pane n unreadable") {
		t.Errorf("status %q, want the unreadable pane noted", got)
	}
	for y := 0; y < g.H-1; y++ {
		if got := strings.Trim(g.RowText(y)[strings.LastIndex(g.RowText(y), "│")+len("│"):], " "); got != "" {
			t.Errorf("unreadable pane row %d drawn: %q", y, got)
		}
	}
	if got, want := r.finish(t, "ba"), []string{"copy /left"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTypingAnUnreadablePanesLabelSaysSo(t *testing.T) {
	captures := map[string]fixture.Capture{
		"w1:p1": screenOf("target /one"),
		"w1:p2": screenOf("lower /left"),
	}
	failing := map[string]error{"w1:p3": errors.New("pane_not_found")}
	t.Run("hint mode", func(t *testing.T) {
		r := startSplitWith(t, session.ModeHint, rightTall, "", captures, failing, nil)
		r.arrived(t, 2)
		if got, want := statusText(r.press(t, "n")), "HINT    pane n unreadable"; !strings.HasPrefix(got, want) {
			t.Errorf("status %q, want it to start %q", got, want)
		}
		if got, want := r.finish(t, "ba"), []string{"copy /left"}; !reflect.DeepEqual(got, want) {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("hint mode, label typed before the read failed", func(t *testing.T) {
		r := startSplitWith(t, session.ModeHint, rightTall, "", captures, failing, nil, "w1:p3")
		r.arrived(t, 1)
		r.press(t, "n")
		if got, want := statusText(r.release(t, "w1:p3")), "HINT  n  pane n unreadable"; !strings.HasPrefix(got, want) {
			t.Errorf("status %q, want it to start %q", got, want)
		}
		r.press(t, "\x7f")
		if got, want := r.finish(t, "ba"), []string{"copy /left"}; !reflect.DeepEqual(got, want) {
			t.Errorf("got %q, want %q", got, want)
		}
	})
	t.Run("line mode", func(t *testing.T) {
		r := startSplitWith(t, session.ModeLine, rightTall, "", captures, failing, nil)
		r.arrived(t, 2)
		if got, want := statusText(r.press(t, "n")), "LINE  n  pane n unreadable"; !strings.HasPrefix(got, want) {
			t.Errorf("status %q after the label, want it to start %q", got, want)
		}
		r.press(t, "1")
		if got, want := statusText(r.press(t, "\r")), "LINE  n1  pane n unreadable"; !strings.HasPrefix(got, want) {
			t.Errorf("status %q after commit, want it to start %q", got, want)
		}
		if got := r.finish(t, "\x03"); got != nil {
			t.Errorf("got %q, want nothing copied", got)
		}
	})
}

func TestEmptyTargetLetsANeighbourBeCopied(t *testing.T) {
	tests := []struct {
		name   string
		mode   session.Mode
		target fixture.Capture
		keys   string
		want   string
	}{
		{name: "hint mode, blank target", mode: session.ModeHint, target: fixture.Capture{}, keys: "na", want: "copy /two"},
		{name: "hint mode, no matches on the target", mode: session.ModeHint, target: screenOf("plain words"), keys: "na", want: "copy /two"},
		{name: "line mode, blank target", mode: session.ModeLine, target: fixture.Capture{}, keys: "n1\r", want: "copy neighbour /two"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := startSplit(t, tt.mode, twoPanes, "", map[string]fixture.Capture{
				"w1:p1": tt.target,
				"w1:p2": screenOf("neighbour /two"),
			})
			r.arrived(t, 1)
			if got := r.finish(t, tt.keys); !reflect.DeepEqual(got, []string{tt.want}) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEmptyStateWaitsForTheLastPaneToArrive(t *testing.T) {
	blank, plain := fixture.Capture{}, screenOf("plain words")
	tests := []struct {
		name             string
		mode             session.Mode
		target, neighbor fixture.Capture
		unreadable       bool
		first, last      string // status row before and after the neighbour arrives
	}{
		{name: "hint mode, neither has a match", mode: session.ModeHint, target: plain, neighbor: plain,
			first: "HINT", last: "no matches"},
		{name: "hint mode, blank target, neighbour has text", mode: session.ModeHint, target: blank, neighbor: plain,
			first: "HINT", last: "no matches"},
		{name: "hint mode, both blank", mode: session.ModeHint, target: blank, neighbor: blank,
			first: "HINT", last: "nothing to copy"},
		{name: "hint mode, blank target, neighbour unreadable", mode: session.ModeHint, target: blank, unreadable: true,
			first: "HINT", last: "nothing to copy"},
		{name: "hint mode, no match on the target, neighbour unreadable", mode: session.ModeHint, target: plain, unreadable: true,
			first: "HINT", last: "no matches"},
		{name: "line mode, both blank", mode: session.ModeLine, target: blank, neighbor: blank,
			first: "LINE", last: "nothing to copy"},
		{name: "line mode, blank target, neighbour unreadable", mode: session.ModeLine, target: blank, unreadable: true,
			first: "LINE", last: "nothing to copy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var failing map[string]error
			if tt.unreadable {
				failing = map[string]error{"w1:p2": errors.New("pane_not_found")}
			}
			r := startSplitWith(t, tt.mode, twoPanes, "", map[string]fixture.Capture{
				"w1:p1": tt.target,
				"w1:p2": tt.neighbor,
			}, failing, nil, "w1:p2")
			if got := statusText(r.arrived(t, 0)); got != tt.first {
				t.Errorf("status %q before the neighbour arrived, want %q", got, tt.first)
			}
			if got := statusText(r.release(t, "w1:p2")); got != tt.last {
				t.Errorf("status %q after it arrived, want %q", got, tt.last)
			}
			if got := r.finish(t, "a"); got != nil {
				t.Errorf("got %q, want nothing copied", got)
			}
		})
	}
}

func TestNeighbourGoingAwayKeepsTheOverlayAndItsText(t *testing.T) {
	for _, ev := range []session.Event{
		session.PaneClosed{Pane: "w1:p2"},
		session.PaneExited{Pane: "w1:p2"},
		session.PaneMoved{Previous: "w1:p2"},
	} {
		t.Run(fmt.Sprintf("%T", ev), func(t *testing.T) {
			events := &fakeEvents{ch: make(chan session.Event)}
			r := startSplitWith(t, session.ModeHint, twoPanes, "", map[string]fixture.Capture{
				"w1:p1": screenOf("target /one"),
				"w1:p2": screenOf("neighbour /two"),
			}, nil, events)
			r.arrived(t, 1)
			events.ch <- ev // unbuffered: the session has taken it
			if got, want := r.finish(t, "na"), []string{"copy /two"}; !reflect.DeepEqual(got, want) {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

func TestNoEmptyStateWhileAnyPaneHasSomething(t *testing.T) {
	// The Target has a Match, the neighbour has none: Hint Mode stays.
	r := startSplit(t, session.ModeHint, twoPanes, "", map[string]fixture.Capture{
		"w1:p1": screenOf("target /one"),
		"w1:p2": screenOf("plain words"),
	}, "w1:p2")
	r.arrived(t, 0)
	if got := statusText(r.release(t, "w1:p2")); got != "HINT" {
		t.Errorf("status %q, want Hint Mode", got)
	}
	if got, want := r.finish(t, "a"), []string{"copy /one"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUnreadableNoteNeverCoversTheInputOrItsError(t *testing.T) {
	// threePanes: w1:p3, labelled "b", could not be read. The note goes
	// on the right; on a narrow overlay it has no room beside the input
	// and the error after it, so it is left out.
	tests := []struct {
		name  string
		mode  session.Mode
		w     int
		typed string
		want  string
	}{
		{name: "line mode error", mode: session.ModeLine, w: 30, typed: "b", want: "LINE  b  pane b unreadable"},
		{name: "hint mode error", mode: session.ModeHint, w: 30, typed: "b", want: "HINT    pane b unreadable"},
		{name: "line mode input", mode: session.ModeLine, w: 20, typed: "12,3", want: "LINE  12,3"},
		{name: "room for both", mode: session.ModeLine, w: 30, typed: "1", want: "LINE  1      pane b unreadable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := layout.Place(threePanes, tt.w, 18)
			var m *session.Machine
			if tt.mode == session.ModeHint {
				m = session.NewHintAt(parse(t, "links"), p)
			} else {
				m = session.NewLineAt(parse(t, "links"), p)
			}
			m.NeighbourUnreadable("w1:p3", errors.New("pane_not_found"))
			for _, r := range tt.typed {
				m.Key(keyOf(r))
			}
			if got := statusText(m.Frame()); got != tt.want {
				t.Errorf("status %q, want %q", got, tt.want)
			}
		})
	}
}
