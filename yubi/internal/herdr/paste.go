package herdr

import (
	"context"
	"time"
)

// pasteTimeout bounds a paste, so a stuck socket cannot hang the overlay.
const pasteTimeout = 5 * time.Second

// Paste implements session.Paster with pane.send_input, text only and no
// keys: herdr brackets the text when the pane has bracketed paste on and
// sends no Enter. The text must already be stripped of control bytes;
// herdr passes it through as is.
func (c *Client) Paste(ctx context.Context, pane, text string) error {
	ctx, cancel := context.WithTimeout(ctx, pasteTimeout)
	defer cancel()
	return c.Call(ctx, "pane.send_input", map[string]any{"pane_id": pane, "text": text}, nil)
}
