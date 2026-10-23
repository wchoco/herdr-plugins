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
	cfg     Config // the session's ids, for telling stale events apart
	set     config.Config
	w, h    int
	scr     screen.Screen
	num     line.Numbering
	input   string // Line Mode input typed so far
	err     string // input error shown next to the input until the next key
	message string // when set, a message-only state: nothing is selectable
	info    bool   // message is information, not an error: drawn plain

	pasteFailed bool // message reports a failed paste; stale exits wait for a key

	hint *hintState // set in Hint Mode

	place  layout.Placement         // where the Target and the other panes are drawn
	others map[string]screen.Screen // other panes read so far, by pane id
}

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

func newLine(scr screen.Screen, p layout.Placement, set config.Config) *Machine {
	m := &Machine{w: p.W, h: p.H, scr: scr, set: set, place: p, num: line.NewNumbering(scr, displayedRows(p))}
	if m.num.Max() == 0 {
		m.inform("nothing to copy")
	}
	return m
}

// newMessage starts a message-only state showing the error msg.
func newMessage(msg string, w, h int) *Machine {
	return &Machine{w: w, h: h, message: msg}
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
	set := config.Default()
	if cfg.Settings != nil {
		set = *cfg.Settings
	}
	ansi, err := src.ReadVisible(ctx, cfg.Target, FormatANSI)
	if err != nil {
		return newMessage("cannot read target: "+err.Error(), w, h)
	}
	p := layout.Place(targetTab(cfg), w, h)
	if cfg.Mode == ModeHint {
		return newHint(screen.ParseANSI(ansi), p, set)
	}
	return newLine(screen.ParseANSI(ansi), p, set)
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

// Resize handles the overlay's new size. Any change ends the session
// silently; yubi does not re-render. A paste failure report is the
// exception: it is drawn again at the new size and waits for a key.
func (m *Machine) Resize(w, h int) []Action {
	if w == m.w && h == m.h {
		return nil
	}
	if m.pasteFailed {
		m.w, m.h = w, h
		return []Action{Redraw{}}
	}
	return []Action{Exit{}}
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
	case bound(m.set.Copy, k):
		return m.commit(false, false)
	case bound(m.set.Joined, k):
		return m.commit(true, false)
	case k.Mod&keys.Alt != 0:
		// Alt on any commit key adds the paste.
		k.Mod &^= keys.Alt
		switch {
		case bound(m.set.Copy, k):
			return m.commit(false, true)
		case bound(m.set.Joined, k):
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
	text := line.Copy(m.scr, rows)
	if joined {
		text = line.JoinedCopy(m.scr, rows, m.set.Separator)
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
		if strings.TrimSpace(m.scr.Rows[r].Text()) != "" {
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
	return m.num.Select(ranges)
}

// Frame renders the current state.
func (m *Machine) Frame() *render.Grid {
	g := render.NewGrid(m.w, m.h)
	if m.h == 0 {
		return g
	}
	status := m.h - 1
	if m.message == "" {
		m.drawOthers(g)
	}
	t := m.place.Target
	if m.message == "" && m.hint != nil {
		m.hint.draw(g, m.scr, t, displayedRows(m.place))
	} else if m.message == "" {
		labelWidth := len(strconv.Itoa(m.num.Max()))
		shown := min(displayedRows(m.place), len(m.scr.Rows))
		for y := 0; y < shown; y++ {
			drawRow(g, t.Y+y, t.X, t.Width, m.scr.Rows[y])
		}
		for _, y := range m.num.Rows(line.Preview(m.input)) {
			g.Select(t.Y+y, t.X, t.Width)
		}
		for y := 0; y < shown; y++ {
			if n := m.num.Of(y); n > 0 {
				g.Label(t.Y+y, t.X, padLeft(strconv.Itoa(n), labelWidth), render.LineNumber)
			}
		}
	}
	g.Fill(status, 0, m.w, render.Status)
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
