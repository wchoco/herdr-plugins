package session

import (
	"context"
	"strconv"
	"strings"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/config"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/keys"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/layout"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/line"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/render"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
)

// Action is what the state machine asks the session loop to do.
type Action interface{ action() }

// Copy puts Text on the clipboard; on success the session ends.
type Copy struct{ Text string }

// Paste copies Text, then pastes it into the Target Pane (Paste Copy); on
// success the session ends.
type Paste struct{ Text string }

// Exit ends the session without copying.
type Exit struct{}

// Redraw draws the current frame again.
type Redraw struct{}

func (Copy) action()   {}
func (Paste) action()  {}
func (Exit) action()   {}
func (Redraw) action() {}

// Machine is the overlay's state machine: key events in, actions out, and
// the frame for the current state.
type Machine struct {
	cfg      Config        // the session's ids, for telling stale events apart
	settings config.Config // from config.toml, or the defaults
	input    string        // Line Mode input typed so far
	err      string        // input error shown next to the input until the next key
	message  string        // when set, a message-only state: nothing is selectable
	info     bool          // message is information, not an error: drawn plain

	pasteFailed bool // message reports a failed paste; stale exits wait for a key

	hint *hintState // set in Hint Mode

	place layout.Placement // where the Visible Panes are drawn
	panes []*visiblePane   // the Visible Panes, the Target Pane first
}

// target returns the Target Pane.
func (m *Machine) target() *visiblePane { return m.panes[0] }

// NewLine starts Line Mode on scr in a w x h overlay. The status row is the
// overlay's bottom row, so the rows above it are the rows yubi displays.
func NewLine(scr screen.Screen, w, h int) *Machine {
	return newLine(scr, layout.Place(nil, w, h), config.Default())
}

// NewLineAt starts Line Mode on scr with the Target Pane and the other
// panes placed by p.
func NewLineAt(scr screen.Screen, p layout.Placement) *Machine {
	return newLine(scr, p, config.Default())
}

func newLine(scr screen.Screen, p layout.Placement, settings config.Config) *Machine {
	m := &Machine{settings: settings, place: p, panes: visiblePanes(p)}
	m.target().arrive(scr, p.H)
	if m.target().num.Max() == 0 {
		m.inform("nothing to copy")
	}
	return m
}

// newMessage starts a message-only state showing the error msg.
func newMessage(msg string, w, h int) *Machine {
	return &Machine{place: layout.Placement{W: w, H: h}, message: msg}
}

// start reads the Target Pane and builds the machine for cfg.
func start(ctx context.Context, cfg Config, src PaneSource, w, h int) *Machine {
	m := build(ctx, cfg, src, w, h)
	m.cfg = cfg
	return m
}

func build(ctx context.Context, cfg Config, src PaneSource, w, h int) *Machine {
	switch cfg.Mode {
	case ModeLine, ModeHint:
	default:
		return newMessage("unknown mode "+strconv.Quote(string(cfg.Mode)), w, h)
	}
	if cfg.SettingsErr != nil {
		return newMessage(cfg.SettingsErr.Error(), w, h)
	}
	settings := config.Default()
	if cfg.Settings != nil {
		settings = *cfg.Settings
	}
	ansi, err := src.ReadVisible(ctx, cfg.Target, FormatANSI)
	if err != nil {
		return newMessage("cannot read target: "+err.Error(), w, h)
	}
	p := layout.Place(targetTab(cfg), w, h)
	if cfg.Mode == ModeHint {
		return newHint(screen.ParseANSI(ansi), p, settings)
	}
	return newLine(screen.ParseANSI(ansi), p, settings)
}

// Fail switches to a message-only state showing the error msg.
func (m *Machine) Fail(msg string) { m.message, m.info = msg, false }

// inform switches to a message-only state showing msg, which is not an
// error.
func (m *Machine) inform(msg string) { m.message, m.info = msg, true }

// FailPaste reports a Paste Copy whose copy worked but whose paste failed.
// The report then outlives events and resizes that would otherwise end the
// session silently: the user must learn the paste did not happen.
func (m *Machine) FailPaste(err error) {
	m.Fail("copied; paste failed: " + err.Error())
	m.pasteFailed = true
}

// Event handles a change in the herdr session. A change that makes the
// render stale ends the session silently, in every state: yubi never
// copies from a stale render or lingers as an orphan pane.
func (m *Machine) Event(ev Event) []Action {
	if stale(m.cfg, ev) && !m.pasteFailed {
		return []Action{Exit{}}
	}
	return nil
}

// Resize handles the overlay's new size. When the tab was resized too
// (tabResized), the Target may have reflowed: the session ends silently,
// as yubi does not re-render. Otherwise herdr resized only the overlay,
// e.g. dropping its scrollbar gutter once yubi is on the alternate screen,
// and the frame is drawn again at the new size with everything in place.
// A paste failure report is always drawn again and waits for a key.
func (m *Machine) Resize(w, h int, tabResized bool) []Action {
	if w == m.place.W && h == m.place.H {
		return nil
	}
	if tabResized && !m.pasteFailed {
		return []Action{Exit{}}
	}
	m.place.W, m.place.H = w, h
	return []Action{Redraw{}}
}

// Key handles one key press.
func (m *Machine) Key(k keys.Key) []Action {
	if m.message != "" {
		return []Action{Exit{}}
	}
	if m.hint != nil {
		return m.hint.key(k)
	}
	switch {
	case k == keys.Key{Code: keys.Escape}, k == keys.Key{Code: 'c', Mod: keys.Ctrl}:
		return []Action{Exit{}}
	case k.Mod == 0 && (k.Code >= '0' && k.Code <= '9' || k.Code == '-' || k.Code == ','):
		return m.edit(m.input + string(k.Code))
	case k == keys.Key{Code: keys.Backspace}:
		if m.input == "" {
			return nil
		}
		return m.edit(m.input[:len(m.input)-1])
	case k == keys.Key{Code: 'u', Mod: keys.Ctrl}:
		return m.edit("")
	case bound(m.settings.Copy, k):
		return m.commit(false, false)
	case bound(m.settings.Joined, k):
		return m.commit(true, false)
	case k.Mod&keys.Alt != 0:
		// Alt on any commit key adds the paste.
		k.Mod &^= keys.Alt
		switch {
		case bound(m.settings.Copy, k):
			return m.commit(false, true)
		case bound(m.settings.Joined, k):
			return m.commit(true, true)
		}
	}
	return nil
}

// bound reports whether k is one of the commit keys ks (in keys.Canon form).
func bound(ks []keys.Key, k keys.Key) bool {
	k = keys.Canon(k)
	for _, b := range ks {
		if b == k {
			return true
		}
	}
	return false
}

// edit replaces the Line Mode input; any shown error goes away.
func (m *Machine) edit(input string) []Action {
	m.input = input
	m.err = ""
	return []Action{Redraw{}}
}

// commit validates the whole input and copies the selected rows, as one
// line when joined, or shows why it cannot; paste pastes the text into the
// Target Pane too. Empty input is a no-op.
func (m *Machine) commit(joined, paste bool) []Action {
	if m.input == "" {
		return nil
	}
	rows, err := m.selection()
	if err != nil {
		m.err = err.Error()
		return []Action{Redraw{}}
	}
	if m.blank(rows) {
		m.err = "nothing to copy"
		return []Action{Redraw{}}
	}
	scr := m.target().scr
	text := line.Copy(scr, rows)
	if joined {
		text = line.JoinedCopy(scr, rows, m.settings.Separator)
	}
	if paste {
		return []Action{Paste{Text: text}}
	}
	return []Action{Copy{Text: text}}
}

// blank reports whether every one of rows is blank, so the selection has
// no text to copy.
func (m *Machine) blank(rows []int) bool {
	for _, r := range rows {
		if strings.TrimSpace(m.target().scr.Rows[r].Text()) != "" {
			return false
		}
	}
	return true
}

func (m *Machine) selection() ([]int, error) {
	ranges, err := line.Parse(m.input)
	if err != nil {
		return nil, err
	}
	return m.target().num.Select(ranges)
}

// drawPanes draws the pane frames as faint rules, then each Visible Pane
// in its grid: the Neighbour Panes read so far dimmed, then the Target
// Pane with the current mode's Hints or Line Numbers.
func (m *Machine) drawPanes(g *render.Grid) {
	for _, f := range m.place.Frames {
		g.Box(f.X, f.Y, f.Width, f.Height, render.Rule)
	}
	for _, p := range m.panes[1:] {
		drawDimmed(g, p)
	}
	if m.hint != nil {
		m.hint.draw(g, m.target(), m.place.H)
	} else {
		m.drawLines(g, m.target())
	}
}

// drawLines draws the displayed rows of p in its grid with their Line
// Numbers, the rows the input selects so far in reverse video.
func (m *Machine) drawLines(g *render.Grid, p *visiblePane) {
	t := p.grid
	labelWidth := len(strconv.Itoa(p.num.Max()))
	shown := min(p.displayed(m.place.H), len(p.scr.Rows))
	for y := 0; y < shown; y++ {
		drawRow(g, t.Y+y, t.X, t.Width, p.scr.Rows[y])
	}
	for _, y := range p.num.Rows(line.Preview(m.input)) {
		g.Select(t.Y+y, t.X, t.Width)
	}
	for y := 0; y < shown; y++ {
		if n := p.num.Of(y); n > 0 {
			g.Label(t.Y+y, t.X, padLeft(strconv.Itoa(n), labelWidth), render.LineNumber)
		}
	}
}

// Frame renders the current state.
func (m *Machine) Frame() *render.Grid {
	g := render.NewGrid(m.place.W, m.place.H)
	if g.H == 0 {
		return g
	}
	status := g.H - 1
	if m.message == "" {
		m.drawPanes(g)
	}
	g.Fill(status, 0, g.W, render.Status)
	text, errText := m.statusText()
	if m.message != "" && !m.info {
		g.Label(status, 0, text, render.Error)
	} else {
		g.Label(status, 0, text, render.Status)
	}
	if errText != "" {
		// The status text is ASCII in Line Mode, so its width is its length.
		g.Label(status, len(text)+2, errText, render.Error)
	}
	if m.message == "" && m.hint != nil && m.hint.paste {
		// Hint Mode's status text is ASCII too.
		g.Label(status, len(text)+2, "paste", render.Paste)
	}
	return g
}

// statusText returns the status row's text and the error shown after it.
func (m *Machine) statusText() (text, errText string) {
	if m.message != "" {
		return m.message, ""
	}
	if m.hint != nil {
		return m.hint.status(), ""
	}
	return "LINE  " + m.input, m.err
}

func padLeft(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return strings.Repeat(" ", w-len(s)) + s
}
