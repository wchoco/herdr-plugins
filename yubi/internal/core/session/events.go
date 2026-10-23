package session

import "context"

// Event is a change in the herdr session around the overlay, as the
// EventStream reports it. Each type mirrors one herdr event.
type Event interface{ sessionEvent() }

// PaneClosed: a pane was closed (pane_closed).
type PaneClosed struct{ Pane string }

// PaneExited: a pane's process exited (pane_exited). herdr sends no
// PaneClosed for it.
type PaneExited struct{ Pane string }

// PaneMoved: a pane moved and got a new id; Previous is its old id
// (pane_moved).
type PaneMoved struct{ Previous string }

// PaneFocused: a pane got focus (pane_focused).
type PaneFocused struct{ Pane string }

// TabFocused: a tab got focus (tab_focused).
type TabFocused struct{ Tab string }

// WorkspaceFocused: a workspace got focus (workspace_focused).
type WorkspaceFocused struct{ Workspace string }

// LayoutUpdated: a tab's layout changed (layout_updated).
type LayoutUpdated struct {
	Tab    string
	Zoomed bool
}

func (PaneClosed) sessionEvent()       {}
func (PaneExited) sessionEvent()       {}
func (PaneMoved) sessionEvent()        {}
func (PaneFocused) sessionEvent()      {}
func (TabFocused) sessionEvent()       {}
func (WorkspaceFocused) sessionEvent() {}
func (LayoutUpdated) sessionEvent()    {}

// EventStream watches the herdr session.
type EventStream interface {
	// Subscribe starts watching. Events arrive on the channel until ctx is
	// done or the subscription ends, when the channel is closed. An error
	// means the subscription was refused or herdr is unreachable.
	Subscribe(ctx context.Context) (<-chan Event, error)
}

// stale reports whether ev makes the overlay's render stale, which ends the
// session: the Target Pane closed, exited or moved, the overlay's tab lost
// its zoom, or focus left the overlay. Ids the Config leaves empty are not
// checked.
func stale(cfg Config, ev Event) bool {
	switch ev := ev.(type) {
	case PaneClosed:
		return ev.Pane == cfg.Target
	case PaneExited:
		return ev.Pane == cfg.Target
	case PaneMoved:
		return ev.Previous == cfg.Target || ev.Previous == cfg.Overlay
	case PaneFocused:
		return cfg.Overlay != "" && ev.Pane != cfg.Overlay
	case TabFocused:
		return cfg.Tab != "" && ev.Tab != cfg.Tab
	case WorkspaceFocused:
		return cfg.Workspace != "" && ev.Workspace != cfg.Workspace
	case LayoutUpdated:
		return cfg.Tab != "" && ev.Tab == cfg.Tab && !ev.Zoomed
	}
	return false
}
