package session

import (
	"context"

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
// background. It is drawn dimmed in that pane's grid; a pane that is not
// a Neighbour Pane is ignored.
func (m *Machine) Neighbour(pane string, scr screen.Screen) []Action {
	p := m.neighbour(pane)
	if p == nil {
		return nil
	}
	p.arrive(scr, m.place.H)
	return []Action{Redraw{}}
}

// NeighbourUnreadable tells the machine that a Neighbour Pane could not be
// read. The pane stays blank and nothing is shown; a pane that is not a
// Neighbour Pane is ignored.
func (m *Machine) NeighbourUnreadable(pane string, err error) []Action {
	if p := m.neighbour(pane); p != nil {
		p.fail(err)
	}
	return nil
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
	r := p.grid
	for y := 0; y < r.Height && y < len(p.scr.Rows); y++ {
		drawRow(g, r.Y+y, r.X, r.Width, p.scr.Rows[y])
		g.Restyle(r.Y+y, r.X, r.Width, render.Dim)
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

// readOthers reads every Neighbour Pane once, in the background, and
// delivers each result, a failed read included. The reads end when ctx is
// done; nothing waits for them.
func readOthers(ctx context.Context, src PaneSource, others []layout.Pane) <-chan neighbourRead {
	out := make(chan neighbourRead, len(others)) // never blocks a reader
	for _, o := range others {
		go func() {
			ansi, err := src.ReadVisible(ctx, o.ID, FormatANSI)
			if err != nil {
				out <- neighbourRead{pane: o.ID, err: err}
				return
			}
			out <- neighbourRead{pane: o.ID, scr: screen.ParseANSI(ansi)}
		}()
	}
	return out
}
