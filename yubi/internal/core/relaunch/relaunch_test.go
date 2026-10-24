package relaunch_test

import (
	"context"
	"errors"
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/relaunch"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/session"
)

// fakePanes answers pane get from a fixed set of live panes.
type fakePanes struct {
	live  map[string]bool
	err   error
	asked []string
}

func (f *fakePanes) Exists(_ context.Context, pane string) (bool, error) {
	f.asked = append(f.asked, pane)
	if f.err != nil {
		return false, f.err
	}
	return f.live[pane], nil
}

func TestDecide(t *testing.T) {
	open := &relaunch.Record{Overlay: "w1:p2", Target: "w1:p1", Mode: session.ModeHint}
	zoomed := &relaunch.Record{Overlay: "w1:p2", Target: "w1:p1", Mode: session.ModeHint, Zoomed: true}
	tests := []struct {
		name    string
		rec     *relaunch.Record
		live    []string
		focused string
		mode    session.Mode
		want    relaunch.Plan
	}{
		{
			name:    "no record opens on the focused pane",
			focused: "w1:p1", mode: session.ModeHint,
			want: relaunch.Plan{Kind: relaunch.Open, Target: "w1:p1"},
		},
		{
			name: "same mode on the open overlay closes it",
			rec:  open, live: []string{"w1:p1", "w1:p2"},
			focused: "w1:p2", mode: session.ModeHint,
			want: relaunch.Plan{Kind: relaunch.Close, Overlay: "w1:p2", Target: "w1:p1"},
		},
		{
			name: "closing an overlay opened on a zoomed tab zooms it again",
			rec:  zoomed, live: []string{"w1:p1", "w1:p2"},
			focused: "w1:p2", mode: session.ModeHint,
			want: relaunch.Plan{Kind: relaunch.Close, Overlay: "w1:p2", Target: "w1:p1", Zoomed: true},
		},
		{
			name: "switching an overlay opened on a zoomed tab zooms it again",
			rec:  zoomed, live: []string{"w1:p1", "w1:p2"},
			focused: "w1:p2", mode: session.ModeLine,
			want: relaunch.Plan{Kind: relaunch.Switch, Overlay: "w1:p2", Target: "w1:p1", Zoomed: true},
		},
		{
			name: "other mode on the open overlay switches on the original Target Pane",
			rec:  open, live: []string{"w1:p1", "w1:p2"},
			focused: "w1:p2", mode: session.ModeLine,
			want: relaunch.Plan{Kind: relaunch.Switch, Overlay: "w1:p2", Target: "w1:p1"},
		},
		{
			// The overlay ended just before the press (herdr still names
			// it as focused): it is already closed, so the toggle is done.
			name: "same mode on an overlay that just ended does nothing",
			rec:  open, live: []string{"w1:p1"},
			focused: "w1:p2", mode: session.ModeHint,
			want: relaunch.Plan{Kind: relaunch.None},
		},
		{
			name: "other mode on an overlay that just ended opens on the original Target Pane",
			rec:  open, live: []string{"w1:p1"},
			focused: "w1:p2", mode: session.ModeLine,
			want: relaunch.Plan{Kind: relaunch.Open, Target: "w1:p1"},
		},
		{
			name: "a recorded overlay that is not focused is left alone",
			rec:  open, live: []string{"w1:p1", "w1:p2", "w1:p3"},
			focused: "w1:p3", mode: session.ModeHint,
			want: relaunch.Plan{Kind: relaunch.Open, Target: "w1:p3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			panes := &fakePanes{live: map[string]bool{}}
			for _, p := range tt.live {
				panes.live[p] = true
			}
			got, err := relaunch.Decide(context.Background(), panes, tt.rec, tt.focused, tt.mode)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecideVerifiesTheRecordedOverlay(t *testing.T) {
	panes := &fakePanes{live: map[string]bool{"w1:p2": true}}
	rec := &relaunch.Record{Overlay: "w1:p2", Target: "w1:p1", Mode: session.ModeHint}
	if _, err := relaunch.Decide(context.Background(), panes, rec, "w1:p2", session.ModeHint); err != nil {
		t.Fatal(err)
	}
	if len(panes.asked) != 1 || panes.asked[0] != "w1:p2" {
		t.Errorf("pane get asked for %v, want [w1:p2]", panes.asked)
	}
}

func TestDecidePaneGetFailure(t *testing.T) {
	boom := errors.New("socket unreachable")
	panes := &fakePanes{err: boom}
	rec := &relaunch.Record{Overlay: "w1:p2", Target: "w1:p1", Mode: session.ModeHint}
	_, err := relaunch.Decide(context.Background(), panes, rec, "w1:p2", session.ModeHint)
	if !errors.Is(err, boom) {
		t.Fatalf("got %v, want %v", err, boom)
	}
}
