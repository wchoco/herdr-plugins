// Package session is yubi's overlay session: the state machine that turns
// key events into frames, copies and exit, and the loop that drives it
// through the adapter interfaces defined here.
//
// session is core: it does no I/O of its own and never imports the herdr
// adapter. The adapter implements PaneSource, Clipboard and Terminal.
package session

import (
	"context"
	"fmt"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/render"
)

// Format is the format of a pane read.
type Format int

const (
	FormatText Format = iota // plain text, trailing whitespace trimmed
	FormatANSI               // with SGR, written trailing spaces kept
)

// PaneSource reads the Target Pane.
type PaneSource interface {
	// ReadVisible returns the pane's visible screen as
	// `pane read --source visible` does.
	ReadVisible(ctx context.Context, pane string, f Format) (string, error)
}

// Clipboard puts text on the user's clipboard.
type Clipboard interface {
	Copy(text string) error
}

// Terminal is yubi's own overlay pane.
type Terminal interface {
	// Size returns the overlay's size in cells.
	Size() (w, h int, err error)
	// Input delivers raw key bytes, one read per value. It is closed when
	// the input ends.
	Input() <-chan []byte
	// Draw shows a full frame.
	Draw(frame *render.Grid) error
}

// Mode is the selection mode a session starts in.
type Mode string

const (
	ModeHint Mode = "hint"
	ModeLine Mode = "line"
)

// Config says what a session works on.
type Config struct {
	Mode   Mode
	Target string // Target Pane id
}

// Deps are the adapters a session runs with.
type Deps struct {
	Source    PaneSource
	Clipboard Clipboard
	Terminal  Terminal
}

// Run runs one overlay session until the user copies, cancels or the input
// ends. Errors the user should see are shown in the status row instead of
// being returned; Run returns an error only when it cannot draw at all.
func Run(ctx context.Context, cfg Config, d Deps) error {
	w, h, err := d.Terminal.Size()
	if err != nil {
		return fmt.Errorf("terminal size: %w", err)
	}
	m := start(ctx, cfg, d.Source, w, h)
	if err := d.Terminal.Draw(m.Frame()); err != nil {
		return err
	}
	for {
		var b []byte
		var ok bool
		select {
		case <-ctx.Done():
			return nil
		case b, ok = <-d.Terminal.Input():
		}
		if !ok {
			return nil
		}
		redraw := false
		for _, k := range decode(b) {
			for _, a := range m.Key(k) {
				switch a := a.(type) {
				case Copy:
					if err := d.Clipboard.Copy(a.Text); err != nil {
						m.Fail("copy failed: " + err.Error())
					} else {
						return nil
					}
					redraw = true
				case Exit:
					return nil
				case Redraw:
					redraw = true
				}
			}
		}
		if redraw {
			if err := d.Terminal.Draw(m.Frame()); err != nil {
				return err
			}
		}
	}
}
