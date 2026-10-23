package session

import (
	"context"
	"strconv"
	"strings"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/keys"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/line"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/render"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
)

// Action is what the state machine asks the session loop to do.
type Action interface{ action() }

// Copy puts Text on the clipboard; on success the session ends.
type Copy struct{ Text string }

// Exit ends the session without copying.
type Exit struct{}

// Redraw draws the current frame again.
type Redraw struct{}

func (Copy) action()   {}
func (Exit) action()   {}
func (Redraw) action() {}

var decode = keys.Decode

// Machine is the overlay's state machine: key events in, actions out, and
// the frame for the current state.
type Machine struct {
	w, h    int
	scr     screen.Screen
	num     line.Numbering
	input   string // Line Mode input typed so far
	err     string // input error shown next to the input until the next key
	message string // when set, a message-only state: nothing is selectable

	hint *hintState // set in Hint Mode
}

// NewLine starts Line Mode on scr in a w x h overlay. The status row is the
// overlay's bottom row, so the rows above it are the rows yubi displays.
func NewLine(scr screen.Screen, w, h int) *Machine {
	m := &Machine{w: w, h: h, scr: scr, num: line.NewNumbering(scr, h-1)}
	if m.num.Max() == 0 {
		m.message = "nothing to copy"
	}
	return m
}

// newMessage starts a message-only state.
func newMessage(msg string, w, h int) *Machine {
	return &Machine{w: w, h: h, message: msg}
}

// start reads the Target Pane and builds the machine for cfg.
func start(ctx context.Context, cfg Config, src PaneSource, w, h int) *Machine {
	switch cfg.Mode {
	case ModeLine, ModeHint:
	default:
		return newMessage("unknown mode "+strconv.Quote(string(cfg.Mode)), w, h)
	}
	ansi, err := src.ReadVisible(ctx, cfg.Target, FormatANSI)
	if err != nil {
		return newMessage("cannot read target: "+err.Error(), w, h)
	}
	if cfg.Mode == ModeHint {
		return NewHint(screen.ParseANSI(ansi), w, h)
	}
	return NewLine(screen.ParseANSI(ansi), w, h)
}

// Fail switches to a message-only state showing msg.
func (m *Machine) Fail(msg string) { m.message = msg }

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
	case k == keys.Key{Code: keys.Enter}:
		return m.commit()
	}
	return nil
}

// edit replaces the Line Mode input; any shown error goes away.
func (m *Machine) edit(input string) []Action {
	m.input = input
	m.err = ""
	return []Action{Redraw{}}
}

// commit validates the whole input and copies the selected rows, or shows
// why it cannot. Empty input is a no-op.
func (m *Machine) commit() []Action {
	if m.input == "" {
		return nil
	}
	rows, err := m.selection()
	if err != nil {
		m.err = err.Error()
		return []Action{Redraw{}}
	}
	return []Action{Copy{Text: line.Copy(m.scr, rows)}}
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
	if m.message == "" && m.hint != nil {
		m.hint.draw(g, m.scr, status)
	} else if m.message == "" {
		labelWidth := len(strconv.Itoa(m.num.Max()))
		for y := 0; y < status && y < len(m.scr.Rows); y++ {
			g.DrawRow(y, 0, m.scr.Rows[y])
		}
		for _, y := range m.num.Rows(line.Preview(m.input)) {
			g.Select(y)
		}
		for y := 0; y < status && y < len(m.scr.Rows); y++ {
			if n := m.num.Of(y); n > 0 {
				g.Label(y, 0, padLeft(strconv.Itoa(n), labelWidth), render.LineNumber)
			}
		}
	}
	g.Fill(status, 0, m.w, render.Status)
	text, errText := m.statusText()
	g.Label(status, 0, text, render.Status)
	if errText != "" {
		// The status text is ASCII in Line Mode, so its width is its length.
		g.Label(status, len(text)+2, errText, render.Error)
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
