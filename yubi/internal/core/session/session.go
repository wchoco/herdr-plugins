// Package session is yubi's overlay session: the state machine that turns
// key events into frames, copies and exit, and the loop that drives it
// through the adapter interfaces defined here.
//
// session is core: it does no I/O of its own and never imports the herdr
// adapter. The adapter implements PaneSource, Clipboard, Terminal and
// EventStream.
package session

import (
	"context"
	"fmt"
	"time"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/config"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/keys"
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
	// Resized signals that the overlay may have changed size (SIGWINCH).
	Resized() <-chan struct{}
	// Draw shows a full frame.
	Draw(frame *render.Grid) error
}

// Clock is the time source for the lone-Esc timeout.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

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

	// The overlay's own pane, tab and workspace ids. Focus moving off any
	// of them ends the session; an empty id is not checked.
	Overlay, Tab, Workspace string

	// Settings from config.toml; nil means the defaults. When SettingsErr
	// is set the config is invalid: the session only shows the error.
	Settings    *config.Config
	SettingsErr error
}

// Deps are the adapters a session runs with.
type Deps struct {
	Source    PaneSource
	Clipboard Clipboard
	Terminal  Terminal
	Events    EventStream // nil: the session is not watched
	Clock     Clock       // nil means the system clock
}

// Run runs one overlay session until the user copies or cancels, the input
// ends, or the session goes stale (see Machine.Event and Machine.Resize).
// Errors the user should see are shown in the status row instead of being
// returned; Run returns an error only when it cannot draw at all.
func Run(ctx context.Context, cfg Config, d Deps) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // ends the subscription
	w, h, err := d.Terminal.Size()
	if err != nil {
		return fmt.Errorf("terminal size: %w", err)
	}
	// Subscribe before reading, so nothing that happens after the read
	// is missed.
	var events <-chan Event
	var m *Machine
	if d.Events != nil {
		if events, err = d.Events.Subscribe(ctx); err != nil {
			m = newMessage("cannot watch session: "+err.Error(), w, h)
			m.cfg = cfg
		}
	}
	if m == nil {
		m = start(ctx, cfg, d.Source, w, h)
	}
	if err := d.Terminal.Draw(m.Frame()); err != nil {
		return err
	}
	clock := d.Clock
	if clock == nil {
		clock = realClock{}
	}
	dec := keys.NewDecoder(keys.EscTimeout)
	redraw := false
	// apply carries out actions; it reports whether the session is over.
	apply := func(actions []Action) bool {
		for _, a := range actions {
			switch a := a.(type) {
			case Copy:
				err := d.Clipboard.Copy(a.Text)
				if err == nil {
					return true
				}
				m.Fail("copy failed: " + err.Error())
				redraw = true
			case Exit:
				return true
			case Redraw:
				redraw = true
			}
		}
		return false
	}
	// press applies decoded keys; it reports whether the session is over.
	press := func(ks []keys.Key) bool {
		for _, k := range ks {
			if apply(m.Key(k)) {
				return true
			}
		}
		return false
	}
	for {
		redraw = false
		// A lone Esc from a legacy terminal waits for its timeout.
		var timer <-chan time.Time
		if deadline, ok := dec.Deadline(); ok {
			timer = clock.After(deadline.Sub(clock.Now()))
		}
		select {
		case <-ctx.Done():
			return nil
		case b, ok := <-d.Terminal.Input():
			if !ok {
				return nil
			}
			if press(dec.Feed(b, clock.Now())) {
				return nil
			}
		case <-timer:
			if press(dec.Expire(clock.Now())) {
				return nil
			}
		case ev, ok := <-events:
			// Without the subscription yubi cannot tell whether its
			// render is stale, so its end ends the session too.
			if !ok || apply(m.Event(ev)) {
				return nil
			}
		case <-d.Terminal.Resized():
			w, h, err := d.Terminal.Size()
			if err != nil || apply(m.Resize(w, h)) {
				return nil
			}
		}
		if redraw {
			if err := d.Terminal.Draw(m.Frame()); err != nil {
				return err
			}
		}
	}
}
