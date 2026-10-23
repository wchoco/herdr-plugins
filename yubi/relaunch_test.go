package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/session"
	"github.com/wchoco/herdr-plugins/yubi/internal/herdr"
)

// overlay is an open overlay in fakeHerdr.
type overlay struct {
	target string
	mode   session.Mode
}

// fakeHerdr is one tab with a Target Pane that yubi overlays: opening an
// overlay focuses it, closing one gives focus back to its Target Pane, as
// herdr's overlay placement does.
type fakeHerdr struct {
	focused  string
	panes    map[string]bool // live panes
	overlays map[string]overlay
	next     int
}

func newFakeHerdr() *fakeHerdr {
	return &fakeHerdr{focused: "w1:p1", panes: map[string]bool{"w1:p1": true}, overlays: map[string]overlay{}, next: 2}
}

func (f *fakeHerdr) Exists(_ context.Context, pane string) (bool, error) {
	return f.panes[pane], nil
}

func (f *fakeHerdr) CloseOverlay(_ context.Context, pane string) error {
	o, ok := f.overlays[pane]
	if !ok {
		return &herdr.APIError{Code: "plugin_pane_not_found"}
	}
	delete(f.overlays, pane)
	delete(f.panes, pane)
	f.focused = o.target
	return nil
}

// opener returns the open function launch gets for mode.
func (f *fakeHerdr) opener(mode session.Mode) func(context.Context, string) (string, error) {
	return func(_ context.Context, target string) (string, error) {
		id := fmt.Sprintf("w1:p%d", f.next)
		f.next++
		f.panes[id] = true
		f.overlays[id] = overlay{target: target, mode: mode}
		f.focused = id
		return id, nil
	}
}

// press runs the action for mode the way a yubi binding does: on herdr's
// focused pane.
func (f *fakeHerdr) press(t *testing.T, dir string, mode session.Mode) {
	t.Helper()
	if err := launch(context.Background(), f, dir, f.focused, mode, f.opener(mode)); err != nil {
		t.Fatal(err)
	}
}

func TestLaunchTogglesTheOverlay(t *testing.T) {
	for _, mode := range []session.Mode{session.ModeHint, session.ModeLine} {
		t.Run(string(mode), func(t *testing.T) {
			f, dir := newFakeHerdr(), t.TempDir()
			f.press(t, dir, mode)
			if len(f.overlays) != 1 {
				t.Fatalf("after one press: %d overlays, want 1", len(f.overlays))
			}
			f.press(t, dir, mode)
			if len(f.overlays) != 0 {
				t.Fatalf("after two presses: %d overlays, want 0", len(f.overlays))
			}
			if f.focused != "w1:p1" {
				t.Errorf("focus on %s, want the Target Pane w1:p1", f.focused)
			}
			// A third press opens it again.
			f.press(t, dir, mode)
			if len(f.overlays) != 1 {
				t.Fatalf("after three presses: %d overlays, want 1", len(f.overlays))
			}
		})
	}
}

func TestLaunchSwitchesModeOnTheOriginalTarget(t *testing.T) {
	f, dir := newFakeHerdr(), t.TempDir()
	f.press(t, dir, session.ModeHint)
	f.press(t, dir, session.ModeLine)
	want := map[string]overlay{f.focused: {target: "w1:p1", mode: session.ModeLine}}
	if fmt.Sprint(f.overlays) != fmt.Sprint(want) {
		t.Errorf("overlays %v, want %v", f.overlays, want)
	}
	// And back again, still on the original Target Pane.
	f.press(t, dir, session.ModeHint)
	want = map[string]overlay{f.focused: {target: "w1:p1", mode: session.ModeHint}}
	if fmt.Sprint(f.overlays) != fmt.Sprint(want) {
		t.Errorf("overlays %v, want %v", f.overlays, want)
	}
}

func TestLaunchIgnoresALeftoverRecord(t *testing.T) {
	tests := []struct {
		name  string
		state string
	}{
		{name: "overlay gone", state: `{"overlay":"w1:p7","target":"w1:p1","mode":"hint"}`},
		{name: "unreadable", state: `{"overlay":`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, dir := newFakeHerdr(), t.TempDir()
			// The crashed run's overlay id is now focused but not a plugin pane.
			f.focused, f.panes["w1:p7"] = "w1:p7", true
			if err := os.WriteFile(filepath.Join(dir, stateFile), []byte(tt.state), 0o600); err != nil {
				t.Fatal(err)
			}
			f.press(t, dir, session.ModeHint)
			if len(f.overlays) != 1 {
				t.Fatalf("%d overlays, want 1", len(f.overlays))
			}
			for _, o := range f.overlays {
				if o.target != "w1:p7" {
					t.Errorf("overlay on %s, want the focused pane w1:p7", o.target)
				}
			}
		})
	}
}

func TestLaunchWithoutStateDir(t *testing.T) {
	f := newFakeHerdr()
	f.press(t, "", session.ModeHint)
	f.press(t, "", session.ModeHint)
	// No record kept: the second press stacks rather than toggles.
	if len(f.overlays) != 2 {
		t.Errorf("%d overlays, want 2", len(f.overlays))
	}
}
