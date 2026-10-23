package herdr

import (
	"bufio"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/session"
)

// subscriptions are the herdr events the overlay watches, by their dotted
// subscription names.
var subscriptions = []string{
	"pane.closed", "pane.exited", "pane.moved", "pane.focused",
	"tab.focused", "workspace.focused", "layout.updated",
}

// subscribeTimeout bounds the wait for herdr to accept the subscription.
const subscribeTimeout = 5 * time.Second

// Subscribe implements session.EventStream with events.subscribe. The
// subscription keeps its own connection open; the channel is closed when
// ctx is done, herdr closes the connection, or herdr reports an error on
// it (e.g. events_lost).
func (c *Client) Subscribe(ctx context.Context) (<-chan session.Event, error) {
	subs := make([]map[string]string, len(subscriptions))
	for i, s := range subscriptions {
		subs[i] = map[string]string{"type": s}
	}
	hctx, cancel := context.WithTimeout(ctx, subscribeTimeout)
	defer cancel()
	conn, r, err := c.request(hctx, "events.subscribe", map[string]any{"subscriptions": subs}, nil)
	if err != nil {
		return nil, err
	}
	conn.SetDeadline(time.Time{})
	ch := make(chan session.Event)
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	go func() {
		defer close(ch)
		defer stop()
		defer conn.Close()
		readEvents(ctx, r, ch)
	}()
	return ch, nil
}

func readEvents(ctx context.Context, r *bufio.Reader, ch chan<- session.Event) {
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return
		}
		ev, ok := decodeEvent(line)
		if !ok {
			return
		}
		if ev == nil {
			continue
		}
		select {
		case ch <- ev:
		case <-ctx.Done():
			return
		}
	}
}

// decodeEvent decodes one pushed line. It returns ok false for a line that
// ends the subscription (an error, or not JSON) and a nil event for events
// yubi does not use.
func decodeEvent(line []byte) (session.Event, bool) {
	var env struct {
		Event string    `json:"event"`
		Error *APIError `json:"error"`
		Data  struct {
			PaneID         string `json:"pane_id"`
			TabID          string `json:"tab_id"`
			WorkspaceID    string `json:"workspace_id"`
			PreviousPaneID string `json:"previous_pane_id"`
			Layout         struct {
				TabID  string `json:"tab_id"`
				Zoomed bool   `json:"zoomed"`
			} `json:"layout"`
		} `json:"data"`
	}
	if err := json.Unmarshal(line, &env); err != nil || env.Error != nil {
		return nil, false
	}
	d := env.Data
	// Lifecycle events arrive snake_case; accept the documented dotted
	// names too.
	switch strings.ReplaceAll(env.Event, ".", "_") {
	case "pane_closed":
		return session.PaneClosed{Pane: d.PaneID}, true
	case "pane_exited":
		return session.PaneExited{Pane: d.PaneID}, true
	case "pane_moved":
		return session.PaneMoved{Previous: d.PreviousPaneID}, true
	case "pane_focused":
		return session.PaneFocused{Pane: d.PaneID}, true
	case "tab_focused":
		return session.TabFocused{Tab: d.TabID}, true
	case "workspace_focused":
		return session.WorkspaceFocused{Workspace: d.WorkspaceID}, true
	case "layout_updated":
		return session.LayoutUpdated{Tab: d.Layout.TabID, Zoomed: d.Layout.Zoomed}, true
	}
	return nil, true
}
