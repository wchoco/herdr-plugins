package herdr

import (
	"context"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/layout"
)

// Layout returns the layout of pane's tab with pane.layout, with pane as
// the Target Pane. Call it before the overlay opens: while the overlay is
// zoomed herdr reports the Target at its hidden rect.
func (c *Client) Layout(ctx context.Context, pane string) (*layout.Tab, error) {
	var res struct {
		Layout layout.Tab `json:"layout"`
	}
	if err := c.Call(ctx, "pane.layout", map[string]any{"pane_id": pane}, &res); err != nil {
		return nil, err
	}
	res.Layout.Target = pane
	return &res.Layout, nil
}
