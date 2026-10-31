package session

import (
	"cmp"
	"slices"
	"strings"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/hint"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/layout"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/line"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
)

// readState is how far a Visible Pane's read has got.
type readState int

const (
	paneUnread     readState = iota // not read yet: drawn blank
	paneRead                        // scr holds its screen
	paneUnreadable                  // its read failed: drawn blank
)

// visiblePane is one Visible Pane as the session holds it, the Target Pane
// included: where its grid is placed, its screen once read, its Line
// Numbers, and in Hint Mode its Matches and Hints.
type visiblePane struct {
	id    string
	grid  layout.Rect // in overlay cells, clipped to the overlay
	label string      // a Neighbour Pane's Pane Label; "" for the Target Pane and unlabelled panes
	read  readState
	err   error // why the read failed, when paneUnreadable

	scr     screen.Screen
	num     line.Numbering    // over the displayed rows
	matches []hint.Match      // on the displayed rows, in reading order; Hint Mode only
	hints   map[string]string // copied text -> Hint, the Pane Label included; Hint Mode only
}

// minTargetLetters is how many letters of the Hint alphabet the Target
// Pane always keeps: Pane Labels never take them.
const minTargetLetters = 2

// visiblePanes returns the Visible Panes of p, the Target Pane first, all
// unread, with their Pane Labels from alphabet. The Neighbour Panes stay
// in p.Others order (tab order, not reading order).
func visiblePanes(p layout.Placement, alphabet string) []*visiblePane {
	ps := []*visiblePane{{grid: p.Target}}
	for _, o := range p.Others {
		ps = append(ps, &visiblePane{id: o.ID, grid: o.Rect})
	}
	labelPanes(ps[1:], alphabet)
	return ps
}

// labelPanes gives the Neighbour Panes ns their Pane Labels, once: in
// layout reading order (top to bottom, then left to right), each takes
// the last letter of alphabet not taken yet. The Target Pane keeps
// minTargetLetters letters, so the panes past the last label that fits
// get none.
func labelPanes(ns []*visiblePane, alphabet string) {
	order := readingOrder(ns)
	// The alphabet is validated to be distinct lowercase ASCII letters.
	n := min(len(order), max(len(alphabet)-minTargetLetters, 0))
	for i, p := range order[:n] {
		p.label = alphabet[len(alphabet)-1-i : len(alphabet)-i]
	}
}

// readingOrder returns a copy of ps in layout reading order: top to
// bottom, then left to right.
func readingOrder(ps []*visiblePane) []*visiblePane {
	order := slices.Clone(ps)
	slices.SortStableFunc(order, func(a, b *visiblePane) int {
		return cmp.Or(cmp.Compare(a.grid.Y, b.grid.Y), cmp.Compare(a.grid.X, b.grid.X))
	})
	return order
}

// targetAlphabet returns the Target Pane's Hint alphabet: alphabet without
// the Pane Labels of panes.
func targetAlphabet(alphabet string, panes []*visiblePane) string {
	for _, p := range panes {
		if p.label != "" {
			alphabet = strings.ReplaceAll(alphabet, p.label, "")
		}
	}
	return alphabet
}

// displayed returns how many of the pane's rows yubi displays in an
// overlay h rows high: the rows of its grid above the status row, which is
// always the overlay's bottom row.
func (p *visiblePane) displayed(h int) int {
	return max(min(p.grid.Height, h-1-p.grid.Y), 0)
}

// displayedRows returns the screen rows yubi displays of the pane.
func (p *visiblePane) displayedRows(h int) []screen.Row {
	return p.scr.Rows[:max(min(p.displayed(h), len(p.scr.Rows)), 0)]
}

// arrive stores the pane's screen and numbers its displayed rows.
func (p *visiblePane) arrive(scr screen.Screen, h int) {
	p.read, p.scr = paneRead, scr
	p.num = line.NewNumbering(scr, p.displayed(h))
}

// fail records that the pane could not be read.
func (p *visiblePane) fail(err error) {
	p.read, p.err = paneUnreadable, err
}

// findHints finds the Matches on the pane's displayed rows and assigns
// them Hints from alphabet, each after the pane's Pane Label.
func (p *visiblePane) findHints(h int, patterns []hint.Pattern, alphabet string) {
	p.matches = hint.Find(p.displayedRows(h), patterns)
	p.hints = hint.Assign(p.matches, alphabet)
	for text, hn := range p.hints {
		p.hints[text] = p.label + hn
	}
}
