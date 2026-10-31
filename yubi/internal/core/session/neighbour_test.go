package session_test

import (
	"context"
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
	cfg := session.Config{Mode: mode, Target: tab.Target, Layout: tab}
	if toml != "" {
		settings, err := config.Parse([]byte(toml))
		if err != nil {
			t.Fatal(err)
		}
		cfg.Settings = &settings
	}
	r := &splitRun{
		src:  &fakeSource{captures: captures, gate: map[string]chan struct{}{}},
		c:    &calls{},
		term: newLiveTerminal(77, 18),
	}
	for _, id := range gated {
		r.src.gate[id] = make(chan struct{})
	}
	done := make(chan error, 1)
	go func() {
		done <- session.Run(context.Background(), cfg, session.Deps{
			Source: r.src, Clipboard: logClipboard{r.c}, Paster: logPaster{r.c}, Terminal: r.term,
		})
	}()
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
