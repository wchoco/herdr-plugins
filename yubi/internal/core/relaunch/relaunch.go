// Package relaunch decides what a yubi binding does when it is pressed
// again while yubi's overlay is open: herdr intercepts the prefix inside
// the overlay and runs the action, whose focused pane is then the overlay
// itself.
//
// The action records each overlay it opens (Record); the next launch
// verifies the record with pane get and either toggles the overlay closed,
// switches it to the other mode on the original Target Pane, or opens a
// fresh overlay. relaunch is core: it does no I/O of its own.
package relaunch

import (
	"context"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/session"
)

// Record is what the action remembers about the overlay it opened.
type Record struct {
	Overlay string       `json:"overlay"` // the overlay's pane id
	Target  string       `json:"target"`  // the Target Pane's id
	Mode    session.Mode `json:"mode"`
	Zoomed  bool         `json:"zoomed,omitempty"` // the tab was zoomed before the overlay opened
}

// Panes is herdr's pane get.
type Panes interface {
	// Exists reports whether the pane still exists; a pane_not_found
	// answer is (false, nil).
	Exists(ctx context.Context, pane string) (bool, error)
}

// Kind is what a launch does.
type Kind int

const (
	Open   Kind = iota // open a fresh overlay on Target
	Close              // close Overlay (toggle)
	Switch             // close Overlay, then open the other mode on Target
	None               // nothing to do: the overlay has already closed
)

// Plan is the outcome of Decide.
type Plan struct {
	Kind    Kind
	Overlay string // the overlay to close (Close, Switch)
	Target  string // the Target Pane to open on (Open, Switch) or to zoom
	// Zoomed asks to zoom Target again once Overlay is closed (Close,
	// Switch): herdr restores the zoom an overlay saved when its process
	// exits, but not when plugin.pane.close closes it.
	Zoomed bool
}

// Decide plans a launch in mode with focused as herdr's focused pane. rec
// is the last recorded overlay, or nil. A record counts only when its
// overlay is the focused pane; otherwise it is stale and a fresh overlay
// opens on the focused pane. A focused overlay that pane get no longer
// finds ended just before the press: the toggle is then already done, and
// the other mode opens on the recorded Target Pane, never on the gone
// overlay.
func Decide(ctx context.Context, panes Panes, rec *Record, focused string, mode session.Mode) (Plan, error) {
	fresh := Plan{Kind: Open, Target: focused}
	if rec == nil || rec.Overlay != focused {
		return fresh, nil
	}
	ok, err := panes.Exists(ctx, rec.Overlay)
	if err != nil {
		return Plan{}, err
	}
	switch {
	case !ok && rec.Mode == mode:
		return Plan{Kind: None}, nil
	case !ok:
		return Plan{Kind: Open, Target: rec.Target}, nil
	case rec.Mode == mode:
		return Plan{Kind: Close, Overlay: rec.Overlay, Target: rec.Target, Zoomed: rec.Zoomed}, nil
	default:
		return Plan{Kind: Switch, Overlay: rec.Overlay, Target: rec.Target, Zoomed: rec.Zoomed}, nil
	}
}
