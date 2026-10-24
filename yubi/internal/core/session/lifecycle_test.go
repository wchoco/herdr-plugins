package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/layout"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/session"
	"github.com/wchoco/herdr-plugins/yubi/internal/fixture"
)

// The ids of a live session: Target Pane w1:p1 in tab w1:t1 of workspace
// w1, the overlay w1:p3 split from it and zoomed.
var liveCfg = session.Config{
	Mode: session.ModeLine, Target: "w1:p1",
	Overlay: "w1:p3", Tab: "w1:t1", Workspace: "w1",
}

type fakeEvents struct {
	ch  chan session.Event
	err error
}

func (e *fakeEvents) Subscribe(context.Context) (<-chan session.Event, error) {
	if e.err != nil {
		return nil, e.err
	}
	return e.ch, nil
}

// live is a running session whose input, events and resizes the test feeds.
type live struct {
	t      *testing.T
	src    *fakeSource
	clip   *fakeClipboard
	term   *fakeTerminal
	events *fakeEvents
	done   chan error
}

// fakeLayouts answers pane.layout with a fixed tab, or fails with err.
type fakeLayouts struct {
	tab *layout.Tab
	err error
}

func (f *fakeLayouts) Layout(context.Context, string) (*layout.Tab, error) {
	return f.tab, f.err
}

func startLive(t *testing.T, cfg session.Config, src *fakeSource, events *fakeEvents) *live {
	t.Helper()
	return startLiveWith(t, cfg, src, events, nil)
}

func startLiveWith(t *testing.T, cfg session.Config, src *fakeSource, events *fakeEvents, layouts session.Layouts) *live {
	t.Helper()
	l := &live{
		t:      t,
		src:    src,
		clip:   &fakeClipboard{},
		term:   &fakeTerminal{w: 120, h: 39, input: make(chan []byte), resized: make(chan struct{})},
		events: events,
		done:   make(chan error, 1),
	}
	deps := session.Deps{Source: l.src, Clipboard: l.clip, Terminal: l.term}
	if events != nil {
		deps.Events = events
	}
	if layouts != nil {
		deps.Layouts = layouts
	}
	go func() { l.done <- session.Run(context.Background(), cfg, deps) }()
	return l
}

func newLive(t *testing.T) *live {
	t.Helper()
	src := &fakeSource{captures: map[string]fixture.Capture{"w1:p1": fixture.Load(t, "cjk")}}
	return startLive(t, liveCfg, src, &fakeEvents{ch: make(chan session.Event)})
}

// send delivers ev; it fails when Run has already returned.
func (l *live) send(ev session.Event) {
	l.t.Helper()
	select {
	case l.events.ch <- ev:
	case err := <-l.done:
		l.done <- err
		l.t.Fatalf("session ended before %#v", ev)
	case <-time.After(time.Second):
		l.t.Fatalf("session did not take %#v", ev)
	}
}

func (l *live) key(b string) {
	l.t.Helper()
	select {
	case l.term.input <- []byte(b):
	case <-time.After(time.Second):
		l.t.Fatalf("session did not take key %q", b)
	}
}

// ended waits for Run to return and checks it ended silently: no copy and
// no frame after the first.
func (l *live) endedSilently() {
	l.t.Helper()
	select {
	case err := <-l.done:
		if err != nil {
			l.t.Fatal(err)
		}
	case <-time.After(time.Second):
		l.t.Fatal("session still running")
	}
	if len(l.clip.copies) != 0 {
		l.t.Errorf("copied %q", l.clip.copies)
	}
	if len(l.term.frames) != 1 {
		l.t.Errorf("drew %d frames, want only the first", len(l.term.frames))
	}
}

// running checks the session is still up by typing a digit, which draws
// one more frame.
func (l *live) running() {
	l.t.Helper()
	l.key("1")
	l.key("\x1b[27u") // Esc ends it so the test leaves nothing behind
	select {
	case <-l.done:
	case <-time.After(time.Second):
		l.t.Fatal("session did not end on Esc")
	}
	if len(l.term.frames) != 2 {
		l.t.Errorf("drew %d frames, want 2", len(l.term.frames))
	}
}

func TestStaleEventsEndTheSessionSilently(t *testing.T) {
	tests := []struct {
		name string
		ev   session.Event
	}{
		{"target closed", session.PaneClosed{Pane: "w1:p1"}},
		{"target exited", session.PaneExited{Pane: "w1:p1"}},
		{"target moved", session.PaneMoved{Previous: "w1:p1"}},
		{"focus moved to the target", session.PaneFocused{Pane: "w1:p1"}},
		{"focus moved to another pane", session.PaneFocused{Pane: "w4:p1"}},
		{"another tab focused", session.TabFocused{Tab: "w1:t2"}},
		{"another workspace focused", session.WorkspaceFocused{Workspace: "w2"}},
		{"tab un-zoomed", session.LayoutUpdated{Tab: "w1:t1", Zoomed: false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newLive(t)
			l.send(tt.ev)
			l.endedSilently()
		})
	}
}

func TestUnrelatedEventsKeepTheSession(t *testing.T) {
	l := newLive(t)
	for _, ev := range []session.Event{
		session.PaneClosed{Pane: "w1:p2"},
		session.PaneExited{Pane: "w2:p1"},
		session.PaneMoved{Previous: "w1:p2"},
		session.PaneFocused{Pane: "w1:p3"},
		session.TabFocused{Tab: "w1:t1"},
		session.WorkspaceFocused{Workspace: "w1"},
		session.LayoutUpdated{Tab: "w1:t1", Zoomed: true}, // split ratio changed
		session.LayoutUpdated{Tab: "w2:t1", Zoomed: false},
	} {
		l.send(ev)
	}
	l.running()
}

func TestEndOfEventStreamEndsTheSessionSilently(t *testing.T) {
	l := newLive(t)
	close(l.events.ch)
	l.endedSilently()
}

func TestResizeEndsTheSessionSilently(t *testing.T) {
	l := newLive(t)
	l.send(session.TabFocused{Tab: "w1:t1"}) // Run has taken its first Size
	l.term.w, l.term.h = 100, 30             // ordered before Run's next Size by the send below
	select {
	case l.term.resized <- struct{}{}:
	case <-time.After(time.Second):
		t.Fatal("session did not take the resize")
	}
	l.endedSilently()
}

// resizeTo resizes the overlay to w x h and signals it.
func (l *live) resizeTo(w, h int) {
	l.t.Helper()
	l.send(session.TabFocused{Tab: "w1:t1"}) // Run has taken its first Size
	l.term.w, l.term.h = w, h                // ordered before Run's next Size by the send below
	select {
	case l.term.resized <- struct{}{}:
	case <-time.After(time.Second):
		l.t.Fatal("session did not take the resize")
	}
}

// liveTab is the Target's tab as captured when the overlay opened.
var liveTab = &layout.Tab{
	Area:  layout.Rect{Width: 123, Height: 41},
	Panes: []layout.Pane{{ID: "w1:p1", Rect: layout.Rect{Width: 123, Height: 41}}},
}

func startLiveInTab(t *testing.T, layouts *fakeLayouts) *live {
	t.Helper()
	cfg := liveCfg
	cfg.Layout = liveTab
	src := &fakeSource{captures: map[string]fixture.Capture{"w1:p1": fixture.Load(t, "cjk")}}
	return startLiveWith(t, cfg, src, &fakeEvents{ch: make(chan session.Event)}, layouts)
}

func TestResizeOfTheOverlayAloneRedrawsAtTheNewSize(t *testing.T) {
	// herdr resizes the overlay on its own, e.g. it drops the scrollbar
	// gutter once yubi is on the alternate screen. The tab keeps its size,
	// so the Target has not reflowed and the session goes on.
	l := startLiveInTab(t, &fakeLayouts{tab: liveTab})
	l.resizeTo(121, 39)
	l.key("1")
	l.key("\x1b[27u")
	select {
	case <-l.done:
	case <-time.After(time.Second):
		l.t.Fatal("session did not end on Esc")
	}
	if len(l.term.frames) != 3 {
		t.Fatalf("drew %d frames, want 3 (first, resize, key)", len(l.term.frames))
	}
	for i, want := range []int{120, 121, 121} {
		if got := l.term.frames[i].W; got != want {
			t.Errorf("frame %d is %d wide, want %d", i, got, want)
		}
	}
}

func TestResizeOfTheTabEndsTheSessionSilently(t *testing.T) {
	tests := []struct {
		name    string
		layouts *fakeLayouts
	}{
		{"tab resized", &fakeLayouts{tab: &layout.Tab{Area: layout.Rect{Width: 100, Height: 30}}}},
		{"tab layout unreadable", &fakeLayouts{err: errString("pane_not_found")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := startLiveInTab(t, tt.layouts)
			l.resizeTo(121, 39)
			l.endedSilently()
		})
	}
}

func TestResizeSignalWithoutSizeChangeKeepsTheSession(t *testing.T) {
	l := newLive(t)
	select {
	case l.term.resized <- struct{}{}:
	case <-time.After(time.Second):
		t.Fatal("session did not take the resize")
	}
	l.running()
}

func TestStaleEventEndsAMessageOnlyStateToo(t *testing.T) {
	src := &fakeSource{captures: map[string]fixture.Capture{"w1:p1": {Name: "blank", ANSI: "\r\n   \r\n"}}}
	l := startLive(t, liveCfg, src, &fakeEvents{ch: make(chan session.Event)})
	l.send(session.PaneClosed{Pane: "w1:p1"})
	l.endedSilently()
}

func TestSubscribeRejectedIsShownAndAnyKeyCloses(t *testing.T) {
	src := &fakeSource{captures: map[string]fixture.Capture{"w1:p1": fixture.Load(t, "cjk")}}
	l := startLive(t, liveCfg, src, &fakeEvents{err: errString("socket unreachable")})
	l.key("x")
	select {
	case <-l.done:
	case <-time.After(time.Second):
		t.Fatal("session still running")
	}
	if got := statusRow(t, l.term); got != "cannot watch session: socket unreachable" {
		t.Errorf("status row %q", got)
	}
	if len(l.src.reads) != 0 {
		t.Errorf("read the target %q after the subscription failed", l.src.reads)
	}
}

func TestNothingToCopyOnBlankScreen(t *testing.T) {
	for name, ansi := range map[string]string{"empty": "", "all blank": "\x1b[0m   \r\n\r\n\x1b[0m\x1b[41m  "} {
		t.Run(name, func(t *testing.T) {
			src := &fakeSource{captures: map[string]fixture.Capture{"w1:p1": {Name: name, ANSI: ansi}}}
			l := startLive(t, liveCfg, src, &fakeEvents{ch: make(chan session.Event)})
			l.key("1") // any key, even one that would select a line
			select {
			case <-l.done:
			case <-time.After(time.Second):
				t.Fatal("session still running")
			}
			if got := statusRow(t, l.term); got != "nothing to copy" {
				t.Errorf("status row %q", got)
			}
			if len(l.clip.copies) != 0 || len(l.term.frames) != 1 {
				t.Errorf("copies %q, %d frames", l.clip.copies, len(l.term.frames))
			}
		})
	}
}

type errString string

func (e errString) Error() string { return string(e) }
