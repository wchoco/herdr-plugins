package session

import (
	"strings"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/config"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/keys"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/layout"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/render"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
)

// hintState is Hint Mode's part of the Machine. The Matches and Hints
// are held per Visible Pane: a Neighbour Pane's Hints start with its Pane
// Label, so typing filters across every pane by prefix, and a typed Pane
// Label of a pane not read yet stays typed.
type hintState struct {
	panes []*visiblePane // the Machine's Visible Panes
	typed string         // Hint characters typed so far
	paste bool           // the first character was uppercase: Paste Copy
}

// NewHint starts Hint Mode on scr in a w x h overlay. Only the rows above
// the status row are displayed, so only they get Hints.
func NewHint(scr screen.Screen, w, h int) *Machine {
	return newHint(scr, layout.Place(nil, w, h), config.Default())
}

// NewHintAt starts Hint Mode on scr with the Target Pane and the other
// panes placed by p. Rows clipped at the bottom get no Hints; Matches are
// found on whole rows, so a Match clipped on the right still copies its
// full text.
func NewHintAt(scr screen.Screen, p layout.Placement) *Machine {
	return newHint(scr, p, config.Default())
}

func newHint(scr screen.Screen, p layout.Placement, settings config.Config) *Machine {
	m := &Machine{settings: settings, place: p, panes: visiblePanes(p, settings.Alphabet)}
	t := m.target()
	t.arrive(scr, p.H)
	if t.num.Max() == 0 {
		m.inform("nothing to copy")
		return m
	}
	t.findHints(p.H, settings.Patterns, targetAlphabet(settings.Alphabet, m.panes))
	if len(t.matches) == 0 {
		m.inform("no matches")
		return m
	}
	m.hint = &hintState{panes: m.panes}
	return m
}

func (s *hintState) key(k keys.Key) []Action {
	k = keys.Canon(k)
	upper := k.Mod == 0 && k.Code >= 'A' && k.Code <= 'Z'
	if upper {
		// Hints are lowercase; only the first character's case counts.
		k.Code += 'a' - 'A'
	}
	switch {
	case k == keys.Key{Code: keys.Escape}, k == keys.Key{Code: 'c', Mod: keys.Ctrl}:
		return []Action{Exit{}}
	case k == keys.Key{Code: keys.Backspace}:
		if s.typed == "" {
			return nil
		}
		s.typed = s.typed[:len(s.typed)-1] // Hints are ASCII letters
		if s.typed == "" {
			s.paste = false
		}
		return []Action{Redraw{}}
	case k.Mod == 0 && k.Code > ' ':
		next := s.typed + string(k.Code)
		paste := s.paste || s.typed == "" && upper
		continues := false
		for _, p := range s.panes {
			if p.label == next && p.read == paneUnread {
				// The pane's Hints show up, filtered, when it arrives.
				continues = true
			}
			for text, h := range p.hints {
				if h == next {
					if paste {
						return []Action{Paste{Text: text}}
					}
					return []Action{Copy{Text: text}}
				}
				if strings.HasPrefix(h, next) {
					continues = true
				}
			}
		}
		if continues {
			s.typed = next
			s.paste = paste
			return []Action{Redraw{}}
		}
	}
	return nil
}

// draw draws the displayed rows of p faint in its grid,
// Matches underlined where visible, and the Hints that still fit the typed
// prefix over the first cells of their Matches. A Hint that does not fit
// before the grid's right edge, because its Match starts too far right or
// off-screen, is right-aligned to the edge instead; several on one row go
// leftward from the edge in Match order.
func (s *hintState) draw(g *render.Grid, p *visiblePane, h int) {
	t, scr, displayed := p.grid, p.scr, p.displayed(h)
	for y := 0; y < displayed; y++ {
		if y < len(scr.Rows) {
			drawRow(g, t.Y+y, t.X, t.Width, scr.Rows[y])
		}
		g.Restyle(t.Y+y, t.X, t.Width, render.Dim)
	}
	for _, m := range p.matches {
		if m.Col < t.Width {
			g.Restyle(t.Y+m.Row, t.X+m.Col, min(m.Width, t.Width-m.Col), render.Match)
		}
	}
	edge := map[int]int{} // row -> where the next right-aligned Hint ends
	for _, m := range p.matches {
		h, ok := p.hints[m.Text]
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
