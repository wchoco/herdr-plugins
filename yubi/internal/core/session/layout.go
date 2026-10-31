package session

import (
	"context"
	"strings"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/layout"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/render"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
)

// targetTab returns the session's tab layout with the Target Pane named
// by the Config, or nil without one.
func targetTab(cfg Config) *layout.Tab {
	if cfg.Layout == nil {
		return nil
	}
	t := *cfg.Layout
	t.Target = cfg.Target
	return &t
}

// Neighbour hands the machine the screen of a Neighbour Pane, read in the
// background. It is drawn in that pane's grid: in Hint Mode a pane with a
// Pane Label gets its Hints, built once from its own Matches and never
// rebuilt, and is otherwise drawn dimmed. A pane that is not a Neighbour
// Pane is ignored.
func (m *Machine) Neighbour(pane string, scr screen.Screen) []Action {
	p := m.neighbour(pane)
	if p == nil {
		return nil
	}
	p.arrive(scr)
	if m.hint != nil && p.label != "" {
		p.findHints(m.settings.Patterns, m.settings.Alphabet)
	}
	m.settle()
	return []Action{Redraw{}}
}

// NeighbourUnreadable tells the machine that a Neighbour Pane could not be
// read. The pane keeps its Pane Label and stays blank, and the status row
// notes it; a pane that is not a Neighbour Pane is ignored.
func (m *Machine) NeighbourUnreadable(pane string, err error) []Action {
	p := m.neighbour(pane)
	if p == nil {
		return nil
	}
	p.fail(err)
	if m.hint != nil {
		// A label typed while the pane was being read leads nowhere.
		m.noteUnreadable(m.hint.typed)
	}
	m.settle()
	return []Action{Redraw{}}
}

// unreadableNote returns the status row's note on the Neighbour Panes
// with a Pane Label that could not be read, in layout reading order, or
// "" when there are none.
func (m *Machine) unreadableNote() string {
	var labels []string
	for _, p := range readingOrder(m.panes[1:]) {
		if p.read == paneUnreadable && p.label != "" {
			labels = append(labels, p.label)
		}
	}
	switch len(labels) {
	case 0:
		return ""
	case 1:
		return "pane " + labels[0] + " unreadable"
	}
	return "panes " + strings.Join(labels, ",") + " unreadable"
}

// neighbour returns the Neighbour Pane with id pane, or nil.
func (m *Machine) neighbour(pane string) *visiblePane {
	if m.panes == nil {
		return nil
	}
	for _, p := range m.panes[1:] {
		if p.id == pane {
			return p
		}
	}
	return nil
}

// drawDimmed draws every row of p's grid dimmed, once p is read; a pane
// not read stays blank.
func drawDimmed(g *render.Grid, p *visiblePane) {
	if p.read != paneRead {
		return
	}
	drawRows(g, p, min(p.grid.Height, len(p.scr.Rows)), true)
}

// drawRows draws the first n rows of p's grid: p's screen rows there and,
// when dim, all n rows dimmed, blank ones included.
func drawRows(g *render.Grid, p *visiblePane, n int, dim bool) {
	r := p.grid
	for y := 0; y < n; y++ {
		if y < len(p.scr.Rows) {
			drawRow(g, r.Y+y, r.X, r.Width, p.scr.Rows[y])
		}
		if dim {
			g.Restyle(r.Y+y, r.X, r.Width, render.Dim)
		}
	}
}

// drawRow draws a screen row at column x of row y, clipped to width
// cells. A wide glyph cut by the clip becomes a space in its style.
func drawRow(g *render.Grid, y, x, width int, r screen.Row) {
	var clipped screen.Row
	col := 0
	for _, gl := range r {
		if col+gl.Width > width {
			if col < width {
				clipped = append(clipped, screen.Glyph{Text: " ", Width: 1, SGR: gl.SGR})
			}
			break
		}
		clipped = append(clipped, gl)
		col += gl.Width
	}
	g.DrawRow(y, x, clipped)
}

// neighbourRead is the result of reading a Neighbour Pane: its screen, or
// why it could not be read.
type neighbourRead struct {
	pane string
	scr  screen.Screen
	err  error
}

// readNeighbours reads every Neighbour Pane once, in the background, and
// delivers each result, a failed read included. The reads end when ctx is
// done; nothing waits for them.
func readNeighbours(ctx context.Context, src PaneSource, neighbours []layout.Pane) <-chan neighbourRead {
	out := make(chan neighbourRead, len(neighbours)) // never blocks a reader
	for _, n := range neighbours {
		go func() {
			ansi, err := src.ReadVisible(ctx, n.ID, FormatANSI)
			if err != nil {
				out <- neighbourRead{pane: n.ID, err: err}
				return
			}
			out <- neighbourRead{pane: n.ID, scr: screen.ParseANSI(ansi)}
		}()
	}
	return out
}
