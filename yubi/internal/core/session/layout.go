package session

import (
	"context"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/layout"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/render"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
)

// displayedRows returns how many of the Target Pane's rows yubi displays:
// the rows of its grid that are above the status row, which is always the
// overlay's bottom row.
func displayedRows(p layout.Placement) int {
	return max(min(p.Target.Height, p.H-1-p.Target.Y), 0)
}

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

// Neighbour hands the machine the screen of another pane in the tab, read
// in the background. It is drawn dimmed at that pane's place; a pane that
// is not one of the other panes is ignored.
func (m *Machine) Neighbour(pane string, scr screen.Screen) []Action {
	for _, o := range m.place.Others {
		if o.ID == pane {
			if m.others == nil {
				m.others = map[string]screen.Screen{}
			}
			m.others[pane] = scr
			return []Action{Redraw{}}
		}
	}
	return nil
}

// drawOthers draws the pane frames as faint rules and the other panes read
// so far dimmed, without labels. A pane not read (yet) stays blank.
func (m *Machine) drawOthers(g *render.Grid) {
	for _, f := range m.place.Frames {
		g.Box(f.X, f.Y, f.Width, f.Height, render.Rule)
	}
	for _, o := range m.place.Others {
		scr, ok := m.others[o.ID]
		if !ok {
			continue
		}
		r := o.Rect
		for y := 0; y < r.Height && y < len(scr.Rows); y++ {
			drawRow(g, r.Y+y, r.X, r.Width, scr.Rows[y])
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

// neighbour is the result of reading another pane.
type neighbour struct {
	pane string
	scr  screen.Screen
}

// readOthers reads every other pane once, in the background, and delivers
// the screens that could be read. A failed read is dropped silently. The
// reads end when ctx is done; nothing waits for them.
func readOthers(ctx context.Context, src PaneSource, others []layout.Pane) <-chan neighbour {
	out := make(chan neighbour, len(others)) // never blocks a reader
	for _, o := range others {
		go func() {
			ansi, err := src.ReadVisible(ctx, o.ID, FormatANSI)
			if err == nil {
				out <- neighbour{o.ID, screen.ParseANSI(ansi)}
			}
		}()
	}
	return out
}
