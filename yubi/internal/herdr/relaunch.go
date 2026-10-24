package herdr

import (
	"context"
	"errors"
)

// Exists implements relaunch.Panes with pane.get: pane_not_found means the
// pane is gone.
func (c *Client) Exists(ctx context.Context, pane string) (bool, error) {
	err := c.Call(ctx, "pane.get", map[string]any{"pane_id": pane}, nil)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Code == "pane_not_found" {
		return false, nil
	}
	return err == nil, err
}

// CloseOverlay closes a plugin pane with plugin.pane.close; herdr restores
// the focus the overlay saved, but leaves the tab un-zoomed even when it
// was zoomed before (herdr 0.9.5). A gone or non-plugin pane is a
// plugin_pane_not_found APIError.
func (c *Client) CloseOverlay(ctx context.Context, pane string) error {
	return c.Call(ctx, "plugin.pane.close", map[string]any{"pane_id": pane}, nil)
}

// Zoom zooms pane's tab onto pane with pane.zoom. A gone pane is a
// pane_not_found APIError.
func (c *Client) Zoom(ctx context.Context, pane string) error {
	return c.Call(ctx, "pane.zoom", map[string]any{"pane_id": pane, "mode": "on"}, nil)
}
