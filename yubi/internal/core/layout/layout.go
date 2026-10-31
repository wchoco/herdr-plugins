// Package layout places the Target Pane and its Neighbour Panes
// inside yubi's full-tab overlay, so that each is drawn exactly where it
// is on screen.
//
// The tab layout comes from herdr's pane.layout, captured before the
// overlay opens: while the overlay is zoomed herdr reports the Target Pane
// at its hidden rect. Layout rects are the panes' outer rects; the grids
// inside them are smaller by the pane frame and the scrollbar gutter.
// herdr 0.9.5 draws those from its ui config (pane_borders,
// pane_outer_borders, pane_scrollbars), which yubi cannot read, so Place
// infers them from how much smaller than the tab the overlay is.
package layout

import "encoding/json"

// Rect is a rectangle of cells.
type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Pane is one pane of a tab.
type Pane struct {
	ID   string `json:"pane_id"`
	Rect Rect   `json:"rect"`
}

// Tab is a tab's layout before the overlay opened. Its JSON is the layout
// object of herdr's pane_layout result; other fields are ignored.
type Tab struct {
	Area   Rect   `json:"area"`
	Target string `json:"focused_pane_id"` // the focused pane, which becomes the Target Pane
	Panes  []Pane `json:"panes"`
	// Zoomed: the Target fills the tab. herdr still reports every pane
	// at its split rect then.
	Zoomed bool `json:"zoomed"`
}

// Decode parses a Tab from JSON.
func Decode(s string) (*Tab, error) {
	var t Tab
	if err := json.Unmarshal([]byte(s), &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// Encode returns t as JSON that Decode reads back.
func Encode(t *Tab) string {
	b, _ := json.Marshal(t) // plain data: cannot fail
	return string(b)
}

// Placement says where things are drawn in a W x H overlay, in overlay
// cells. Target and Others are pane grids, clipped to the overlay; Frames
// are the panes' frames, unclipped (cells outside the overlay are not
// drawn).
type Placement struct {
	W, H   int
	Target Rect
	Others []Pane
	Frames []Rect
}

// Place places tab in a w x h overlay. Without a layout, or when the
// Target Pane is not in it, the Target fills the overlay from its corner.
// A zoomed Target fills it too: herdr draws it framed over the whole tab,
// the overlay's own place.
// A grid that would start above or left of the overlay is drawn from the
// overlay's edge instead: rows are top-anchored and columns left-anchored,
// and what does not fit is clipped on the bottom and right.
func Place(tab *Tab, w, h int) Placement {
	p := Placement{W: w, H: h, Target: Rect{Width: w, Height: h}}
	if tab == nil || tab.Zoomed {
		return p
	}
	found := false
	for _, pn := range tab.Panes {
		found = found || pn.ID == tab.Target
	}
	if !found {
		return p
	}
	// The overlay is a framed or unframed pane over the whole tab area: a
	// frame takes one cell on each side, the scrollbar gutter one column.
	frame := 0
	if tab.Area.Height-h >= 2 {
		frame = 1
	}
	gutter := max(tab.Area.Width-w-2*frame, 0)
	// A lone pane is never framed when the overlay is not (pane_borders
	// "auto" frames only split panes, "off" none); in a split, panes are
	// framed as the overlay is.
	paneFrame := 0
	if len(tab.Panes) > 1 {
		paneFrame = frame
	}
	ox, oy := tab.Area.X+frame, tab.Area.Y+frame
	for _, pn := range tab.Panes {
		r := pn.Rect
		grid := Rect{
			X:      r.X + paneFrame - ox,
			Y:      r.Y + paneFrame - oy,
			Width:  r.Width - 2*paneFrame - gutter,
			Height: r.Height - 2*paneFrame,
		}
		grid = clip(grid, w, h)
		if pn.ID == tab.Target {
			p.Target = grid
		} else {
			p.Others = append(p.Others, Pane{ID: pn.ID, Rect: grid})
		}
		if paneFrame == 1 {
			p.Frames = append(p.Frames, Rect{X: r.X - ox, Y: r.Y - oy, Width: r.Width, Height: r.Height})
		}
	}
	return p
}

// clip moves a grid that starts before the overlay's edge onto it and cuts
// what lies past the overlay's right and bottom edges.
func clip(r Rect, w, h int) Rect {
	r.X, r.Y = max(r.X, 0), max(r.Y, 0)
	r.Width = max(min(r.Width, w-r.X), 0)
	r.Height = max(min(r.Height, h-r.Y), 0)
	return r
}
