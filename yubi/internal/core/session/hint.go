package session

import (
	"strings"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/hint"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/keys"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/line"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/render"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
)

// hintState is Hint Mode's part of the Machine.
type hintState struct {
	matches []hint.Match      // on the displayed rows, in reading order
	hints   map[string]string // copied text -> Hint
	typed   string            // Hint characters typed so far
}

// NewHint starts Hint Mode on scr in a w x h overlay. Only the rows above
// the status row are displayed, so only they get Hints.
func NewHint(scr screen.Screen, w, h int) *Machine {
	m := &Machine{w: w, h: h, scr: scr}
	displayed := scr.Rows[:max(min(h-1, len(scr.Rows)), 0)]
	if line.NewNumbering(scr, h-1).Max() == 0 {
		m.message = "nothing to copy"
		return m
	}
	ms := hint.Find(displayed, hint.Builtins())
	if len(ms) == 0 {
		m.message = "no matches"
		return m
	}
	m.hint = &hintState{matches: ms, hints: hint.Assign(ms, hint.DefaultAlphabet)}
	return m
}

func (s *hintState) key(k keys.Key) []Action {
	switch {
	case k == keys.Key{Code: keys.Escape}, k == keys.Key{Code: 'c', Mod: keys.Ctrl}:
		return []Action{Exit{}}
	case k == keys.Key{Code: keys.Backspace}:
		if s.typed == "" {
			return nil
		}
		s.typed = s.typed[:len(s.typed)-1] // Hints are ASCII letters
		return []Action{Redraw{}}
	case k.Mod == 0 && k.Code > ' ':
		next := s.typed + string(k.Code)
		continues := false
		for text, h := range s.hints {
			if h == next {
				return []Action{Copy{Text: text}}
			}
			if strings.HasPrefix(h, next) {
				continues = true
			}
		}
		if continues {
			s.typed = next
			return []Action{Redraw{}}
		}
	}
	return nil
}

// draw draws the displayed rows faint, Matches underlined, and the Hints
// that still fit the typed prefix over the first cells of their Matches.
func (s *hintState) draw(g *render.Grid, scr screen.Screen, displayed int) {
	for y := 0; y < displayed; y++ {
		if y < len(scr.Rows) {
			g.DrawRow(y, 0, scr.Rows[y])
		}
		g.Restyle(y, 0, g.W, render.Dim)
	}
	for _, m := range s.matches {
		g.Restyle(m.Row, m.Col, m.Width, render.Match)
	}
	for _, m := range s.matches {
		h, ok := s.hints[m.Text]
		if !ok || !strings.HasPrefix(h, s.typed) {
			continue
		}
		g.Label(m.Row, m.Col, s.typed, render.HintTyped)
		g.Label(m.Row, m.Col+len(s.typed), h[len(s.typed):], render.Hint)
	}
}

func (s *hintState) status() string { return "HINT  " + s.typed }
