package session_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/session"
	"github.com/wchoco/herdr-plugins/yubi/internal/fixture"
)

const wrappedURL = "https://example.com/a/very/long/path/segment01/segment02/segment03/segment04/segment05/segment06/segment07/segment08/segment09/segment10/segment11/segment12/segment13/segment14/segment15/segment16/segment17/segment18/segment19/segment20/segment21/segment22/segment23/segment24/segment25/segment26/segment27/segment28/segment29/segment30/"

func TestJoinedCopyCommitKeys(t *testing.T) {
	// wrap: the URL is soft-wrapped over Lines 6 to 4.
	tests := []struct {
		name, key string
	}{
		{name: "legacy ctrl-j", key: "\n"},
		{name: "kitty ctrl-j", key: "\x1b[106;5u"},
		{name: "kitty ctrl-enter", key: "\x1b[13;5u"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := run(t, session.ModeLine, "wrap", nil, "6-4", tt.key)
			if want := []string{wrappedURL}; !reflect.DeepEqual(s.clip.copies, want) {
				t.Errorf("copies %q, want %q", s.clip.copies, want)
			}
		})
	}
}

func TestJoinedCopyKeepsIntermediateTrailingSpaces(t *testing.T) {
	// joined: Line 5 ends in the space the soft wrap fell on, Line 3 has
	// written trailing spaces, Line 2 too but it is the last line.
	s := run(t, session.ModeLine, "joined", nil, "5-2\n")
	want := "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx after the wrapkept   trimmed"
	if !reflect.DeepEqual(s.clip.copies, []string{want}) {
		t.Errorf("copies %q, want %q", s.clip.copies, want)
	}
}

func TestPlainCopyStillSplitsWrappedRows(t *testing.T) {
	s := run(t, session.ModeLine, "joined", nil, "3-2\r")
	if want := []string{"kept\ntrimmed"}; !reflect.DeepEqual(s.clip.copies, want) {
		t.Errorf("copies %q, want %q", s.clip.copies, want)
	}
}

func TestJoinedCopyOnInvalidInputShowsError(t *testing.T) {
	s := run(t, session.ModeLine, "cjk", nil, "9\n")
	if len(s.clip.copies) != 0 {
		t.Errorf("copied %q", s.clip.copies)
	}
	if got := statusRow(t, s.term); got != "LINE  9  no line 9" {
		t.Errorf("status row %q", got)
	}
}

func TestLineModeKeysUnderKittyEncoding(t *testing.T) {
	// Every Line Mode key as yubi's pane receives it after pushing kitty
	// flag 1: text keys, Enter and Backspace stay legacy bytes; Ctrl and
	// Esc chords become CSI u. Enter is also accepted as CSI 13u.
	tests := []struct {
		name   string
		reads  []string
		copies []string
	}{
		{name: "digits range and enter", reads: []string{"7", ",", "3", "-", "2", "\r"}, copies: []string{"user@host:~/proj$ cat notes.txt\nemoji 🎉 party\n全角 reverse"}},
		{name: "csi enter", reads: []string{"1", "\x1b[13u"}, copies: []string{"user@host:~/proj$"}},
		{name: "backspace", reads: []string{"3", "4", "\x7f", "\r"}, copies: []string{"emoji 🎉 party"}},
		{name: "ctrl-u", reads: []string{"4", "\x1b[117;5u", "3", "\r"}, copies: []string{"emoji 🎉 party"}},
		{name: "esc cancels", reads: []string{"3", "\x1b[27u", "\r"}},
		{name: "ctrl-c cancels", reads: []string{"3", "\x1b[99;5u", "\r"}},
		{name: "alt-enter is not a copy", reads: []string{"3", "\x1b[13;3u"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := run(t, session.ModeLine, "cjk", nil, tt.reads...)
			if !reflect.DeepEqual(s.clip.copies, tt.copies) {
				t.Errorf("copies %q, want %q", s.clip.copies, tt.copies)
			}
		})
	}
}

// Scripted input with a fake clock, for the lone-Esc timeout.

type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	armed chan chan time.Time // every channel handed out by After
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(time.Duration) <-chan time.Time {
	ch := make(chan time.Time)
	c.armed <- ch
	return ch
}

// fire advances the clock past any timeout and fires the latest timer.
func (c *fakeClock) fire(t *testing.T, done <-chan error) bool {
	t.Helper()
	var timer chan time.Time
	select {
	case timer = <-c.armed:
	case <-done:
		t.Error("Run ended before the tick")
		return false
	case <-time.After(time.Second):
		t.Error("Run never armed the Esc timer")
		return false
	}
	for len(c.armed) > 0 {
		timer = <-c.armed
	}
	c.mu.Lock()
	c.now = c.now.Add(time.Second)
	now := c.now
	c.mu.Unlock()
	timer <- now
	return true
}

// tick is a scripted event: the Esc timer fires after the timeout passed.
const tick = "<tick>"

// runScripted runs Line Mode on capture, delivering each event only after
// Run has taken the previous one.
func runScripted(t *testing.T, capture string, events ...string) *fakeClipboard {
	t.Helper()
	clock := &fakeClock{now: time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC), armed: make(chan chan time.Time, 64)}
	term := &fakeTerminal{w: 120, h: 39, input: make(chan []byte)}
	clip := &fakeClipboard{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		done <- session.Run(ctx, session.Config{Mode: session.ModeLine, Target: "w1:p1"},
			session.Deps{
				Source:    &fakeSource{captures: map[string]fixture.Capture{"w1:p1": fixture.Load(t, capture)}},
				Clipboard: clip,
				Terminal:  term,
				Clock:     clock,
			})
	}()
	defer cancel()
	for _, ev := range events {
		if ev == tick {
			if !clock.fire(t, done) {
				return clip
			}
			continue
		}
		select {
		case term.input <- []byte(ev):
		case <-done:
			return clip
		}
	}
	close(term.input)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	return clip
}

func TestLegacyLoneEscCancelsAfterTimeout(t *testing.T) {
	clip := runScripted(t, "cjk", "3", "\x1b", tick, "4", "\r")
	if len(clip.copies) != 0 {
		t.Errorf("copied %q", clip.copies)
	}
}

func TestLegacyEscFollowedWithinTimeoutIsAltChord(t *testing.T) {
	// ESC then "4" before the timeout is Alt-4, which Line Mode ignores.
	clip := runScripted(t, "cjk", "3", "\x1b", "4", "\r")
	if want := []string{"emoji 🎉 party"}; !reflect.DeepEqual(clip.copies, want) {
		t.Errorf("copies %q, want %q", clip.copies, want)
	}
}
