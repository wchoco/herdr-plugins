package session

import (
	"github.com/wchoco/herdr-plugins/yubi/internal/core/config"
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
	id   string
	grid layout.Rect // in overlay cells, clipped to the overlay
	read readState
	err  error // why the read failed, when paneUnreadable

	scr     screen.Screen
	num     line.Numbering    // over the displayed rows
	matches []hint.Match      // on the displayed rows, in reading order; Hint Mode only
	hints   map[string]string // copied text -> Hint; Hint Mode only
}

// visiblePanes returns the Visible Panes of p, the Target Pane first, all
// unread.
func visiblePanes(p layout.Placement) []*visiblePane {
	ps := []*visiblePane{{grid: p.Target}}
	for _, o := range p.Others {
		ps = append(ps, &visiblePane{id: o.ID, grid: o.Rect})
	}
	return ps
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
// them Hints from alphabet.
func (p *visiblePane) findHints(h int, settings config.Config) {
	p.matches = hint.Find(p.displayedRows(h), settings.Patterns)
	p.hints = hint.Assign(p.matches, settings.Alphabet)
}
