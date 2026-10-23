package session_test

import (
	"context"
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/render"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/session"
	"github.com/wchoco/herdr-plugins/yubi/internal/fixture"
)

// messageRun runs a session in a 60 x 3 overlay and returns its last frame.
func messageRun(t *testing.T, cfg session.Config, d session.Deps, reads ...string) *render.Grid {
	t.Helper()
	term := newTerminal(60, 3, reads...)
	d.Terminal = term
	if d.Clipboard == nil {
		d.Clipboard = &fakeClipboard{}
	}
	if err := session.Run(context.Background(), cfg, d); err != nil {
		t.Fatal(err)
	}
	if len(term.frames) == 0 {
		t.Fatal("no frame drawn")
	}
	return term.frames[len(term.frames)-1]
}

func oneLine() *fakeSource {
	return &fakeSource{captures: map[string]fixture.Capture{"w1:p1": screenOf("one")}}
}

// statusRoles returns the roles of the status row's first n cells.
func statusRoles(g *render.Grid, n int) []render.Role {
	var rs []render.Role
	for x := 0; x < n && x < g.W; x++ {
		rs = append(rs, g.Cell(g.H-1, x).Role)
	}
	return rs
}

func TestMessageStatesStyleErrorsAsErrorsAndInformationAsStatus(t *testing.T) {
	line := session.Config{Mode: session.ModeLine, Target: "w1:p1"}
	hint := session.Config{Mode: session.ModeHint, Target: "w1:p1"}
	tests := []struct {
		name  string
		cfg   session.Config
		deps  session.Deps
		reads []string
		text  string
		role  render.Role
	}{
		{"unknown mode", session.Config{Mode: "word", Target: "w1:p1"},
			session.Deps{Source: oneLine()}, nil, `unknown mode "word"`, render.Error},
		{"config error", session.Config{Mode: session.ModeLine, Target: "w1:p1", SettingsErr: errString("config: x: unknown key")},
			session.Deps{Source: oneLine()}, nil, "config: x: unknown key", render.Error},
		{"read failure", line,
			session.Deps{Source: &fakeSource{err: errString("pane_not_found")}}, nil, "cannot read target: pane_not_found", render.Error},
		{"watch failure", line,
			session.Deps{Source: oneLine(), Events: &fakeEvents{err: errString("socket unreachable")}}, nil, "cannot watch session: socket unreachable", render.Error},
		{"copy failure", line,
			session.Deps{Source: oneLine(), Clipboard: &fakeClipboard{err: errString("too long")}}, []string{"1\r"}, "copy failed: too long", render.Error},
		{"paste failure", line,
			session.Deps{Source: oneLine(), Paster: logPaster{&calls{pasteErr: errString("gone")}}}, []string{"1", "\x1b\r"}, "copied; paste failed: gone", render.Error},
		{"no matches", hint,
			session.Deps{Source: oneLine()}, nil, "no matches", render.Status},
		{"nothing to copy", line,
			session.Deps{Source: &fakeSource{captures: map[string]fixture.Capture{"w1:p1": {}}}}, nil, "nothing to copy", render.Status},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := messageRun(t, tt.cfg, tt.deps, tt.reads...)
			if got := g.RowText(g.H - 1)[:len(tt.text)]; got != tt.text {
				t.Fatalf("status row %q, want %q", got, tt.text)
			}
			for x, r := range statusRoles(g, len(tt.text)) {
				if r != tt.role {
					t.Fatalf("cell %d has role %d, want %d", x, r, tt.role)
				}
			}
			// The rest of the row stays in the status style.
			if r := g.Cell(g.H-1, len(tt.text)).Role; r != render.Status {
				t.Errorf("cell after the message has role %d", r)
			}
		})
	}
}

func TestMessageRenderGoldens(t *testing.T) {
	tests := []struct {
		name string
		cfg  session.Config
		deps session.Deps
	}{
		{"message-error", session.Config{Mode: session.ModeLine, Target: "w1:p9"},
			session.Deps{Source: &fakeSource{err: errString("pane_not_found")}}},
		{"message-info", session.Config{Mode: session.ModeHint, Target: "w1:p1"},
			session.Deps{Source: oneLine()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			golden(t, tt.name, messageRun(t, tt.cfg, tt.deps).Dump())
		})
	}
}
