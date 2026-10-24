package herdr_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/session"
	"github.com/wchoco/herdr-plugins/yubi/internal/fixture"
	"github.com/wchoco/herdr-plugins/yubi/internal/herdr"
)

// ids names the panes, tab and workspace of one events capture.
type ids struct {
	Workspace, Tab, Target, Sibling, Overlay string
}

func loadIDs(t *testing.T, scenario string) ids {
	t.Helper()
	b, err := os.ReadFile(ndjsonPath("events-" + scenario + ".ids.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v ids
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func ndjsonPath(name string) string {
	return filepath.Join(fixture.Dir(), "herdr-"+fixture.HerdrVersion, "ndjson", name)
}

// replayStream starts a fake herdr socket that takes one subscription: it
// checks the request against the capture's request line (unless check is
// false), then sends the rest of the capture and keeps the connection open
// until the test ends.
func replayStream(t *testing.T, capture string, check bool) string {
	t.Helper()
	b, err := os.ReadFile(ndjsonPath(capture + ".ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	req, rest, _ := strings.Cut(string(b), "\n")
	path := filepath.Join(t.TempDir(), "herdr.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	t.Cleanup(func() { close(done); ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		got, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			t.Errorf("reading request: %v", err)
		} else if check && !sameRequest(t, got, req) {
			t.Errorf("request\n got %s\nwant %s", got, req)
		}
		conn.Write([]byte(rest))
		if strings.Contains(rest, `"error"`) {
			return // herdr closes a rejected subscription
		}
		<-done
	}()
	return path
}

// collect subscribes and gathers events until n arrive or the stream goes
// quiet.
func collect(t *testing.T, path string, n int) []session.Event {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := herdr.NewClient(path).Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var got []session.Event
	for len(got) < n {
		select {
		case ev, ok := <-ch:
			if !ok {
				return got
			}
			got = append(got, ev)
		case <-time.After(time.Second):
			return got
		}
	}
	return got
}

func TestSubscribeDecodesCapturedEvents(t *testing.T) {
	tests := []struct {
		scenario string
		want     func(ids) []session.Event
	}{
		{"target-closed", func(i ids) []session.Event {
			return []session.Event{session.PaneClosed{Pane: i.Target}, session.LayoutUpdated{Tab: i.Tab}}
		}},
		{"target-exited", func(i ids) []session.Event {
			return []session.Event{session.PaneExited{Pane: i.Target}, session.LayoutUpdated{Tab: i.Tab}}
		}},
		{"focus-target", func(i ids) []session.Event {
			return []session.Event{
				session.PaneFocused{Pane: i.Target},
				session.TabFocused{Tab: i.Tab},
				session.WorkspaceFocused{Workspace: i.Workspace},
			}
		}},
		{"sibling-split", func(i ids) []session.Event {
			return []session.Event{session.LayoutUpdated{Tab: i.Tab}}
		}},
		{"split-resized", func(i ids) []session.Event {
			return []session.Event{session.LayoutUpdated{Tab: i.Tab, Zoomed: true}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			want := tt.want(loadIDs(t, tt.scenario))
			got := collect(t, replayStream(t, "events-"+tt.scenario, true), len(want))
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got %#v\nwant %#v", got, want)
			}
		})
	}
}

// The other captures' first event is the one that ends the overlay.
func TestSubscribeDecodesFirstEvent(t *testing.T) {
	tests := []struct {
		scenario string
		want     func(ids) session.Event
	}{
		{"target-moved", func(i ids) session.Event { return session.PaneMoved{Previous: i.Target} }},
		{"tab-focused", func(i ids) session.Event { return session.PaneFocused{Pane: newPane(i, 4)} }},
		{"workspace-focused", func(ids) session.Event { return nil }},
	}
	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			i := loadIDs(t, tt.scenario)
			got := collect(t, replayStream(t, "events-"+tt.scenario, true), 4)
			if len(got) == 0 {
				t.Fatal("no events")
			}
			if want := tt.want(i); want != nil && got[0] != want {
				t.Errorf("first event %#v, want %#v", got[0], want)
			}
			// Every capture ends the session for an overlay with these ids.
			if !endsSession(got, i) {
				t.Errorf("no event in %#v ends the session", got)
			}
		})
	}
}

// newPane is the id of the n-th pane in the capture's workspace.
func newPane(i ids, n int) string {
	return i.Workspace + ":p" + string(rune('0'+n))
}

// endsSession reports whether any event moves focus off the overlay, its
// tab or its workspace, or names the Target as gone.
func endsSession(evs []session.Event, i ids) bool {
	for _, ev := range evs {
		switch ev := ev.(type) {
		case session.PaneFocused:
			if ev.Pane != i.Overlay {
				return true
			}
		case session.TabFocused:
			if ev.Tab != i.Tab {
				return true
			}
		case session.WorkspaceFocused:
			if ev.Workspace != i.Workspace {
				return true
			}
		case session.PaneMoved:
			if ev.Previous == i.Target {
				return true
			}
		}
	}
	return false
}

func TestSubscribeRejected(t *testing.T) {
	path := replayStream(t, "events-subscribe-rejected", false)
	_, err := herdr.NewClient(path).Subscribe(context.Background())
	var apiErr *herdr.APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "pane_not_found" {
		t.Fatalf("got %v, want a pane_not_found APIError", err)
	}
}

func TestSubscribeSocketUnreachable(t *testing.T) {
	c := herdr.NewClient(filepath.Join(t.TempDir(), "missing.sock"))
	_, err := c.Subscribe(context.Background())
	if err == nil || err.Error() != "socket unreachable" {
		t.Fatalf("got %v, want socket unreachable", err)
	}
}

func TestSubscribeChannelClosesWhenHerdrHangsUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "herdr.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		bufio.NewReader(conn).ReadString('\n')
		conn.Write([]byte(`{"id":"x","result":{"type":"subscription_started"}}` + "\n" +
			`{"id":"x","error":{"code":"events_lost","message":"lagged"}}` + "\n"))
		conn.Close()
	}()
	ch, err := herdr.NewClient(path).Subscribe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case ev, ok := <-ch:
		if ok {
			t.Fatalf("got %#v, want the channel closed", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("channel still open")
	}
}
