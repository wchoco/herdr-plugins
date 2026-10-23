package session

import (
	"strings"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/hint"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/keys"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/layout"
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
	return NewHintAt(scr, layout.Place(nil, w, h))
}

// NewHintAt starts Hint Mode on scr with the Target Pane and the other
// panes placed by p. Rows clipped at the bottom get no Hints; Matches are
// found on whole rows, so a Match clipped on the right still copies its
// full text.
func NewHintAt(scr screen.Screen, p layout.Placement) *Machine {
	m := &Machine{w: p.W, h: p.H, scr: scr, place: p}
	n := displayedRows(p)
	displayed := scr.Rows[:max(min(n, len(scr.Rows)), 0)]
	if line.NewNumbering(scr, n).Max() == 0 {
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

// draw draws the displayed rows of the Target Pane faint in its grid t,
// Matches underlined where visible, and the Hints that still fit the typed
// prefix over the first cells of their Matches. A Hint that does not fit
// before the grid's right edge, because its Match starts too far right or
// off-screen, is right-aligned to the edge instead; several on one row go
// leftward from the edge in Match order.
func (s *hintState) draw(g *render.Grid, scr screen.Screen, t layout.Rect, displayed int) {
	for y := 0; y < displayed; y++ {
		if y < len(scr.Rows) {
			drawRow(g, t.Y+y, t.X, t.Width, scr.Rows[y])
		}
		g.Restyle(t.Y+y, t.X, t.Width, render.Dim)
	}
	for _, m := range s.matches {
		if m.Col < t.Width {
			g.Restyle(t.Y+m.Row, t.X+m.Col, min(m.Width, t.Width-m.Col), render.Match)
		}
	}
	edge := map[int]int{} // row -> where the next right-aligned Hint ends
	for _, m := range s.matches {
		h, ok := s.hints[m.Text]
		if !ok {
			continue
		}
		x := m.Col
		if x+len(h) > t.Width {
			end, ok := edge[m.Row]
			if !ok {
				end = t.Width
			}
			x = end - len(h)
			edge[m.Row] = x
		}
		if x < 0 || !strings.HasPrefix(h, s.typed) {
			continue
		}
		g.Label(t.Y+m.Row, t.X+x, s.typed, render.HintTyped)
		g.Label(t.Y+m.Row, t.X+x+len(s.typed), h[len(s.typed):], render.Hint)
	}
}

func (s *hintState) status() string { return "HINT  " + s.typed }
