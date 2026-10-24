package keys_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/keys"
)

// step is one input to a Decoder at a time: a read of bytes, or (when in
// is empty) the session's Esc timer firing.
type step struct {
	at   time.Duration
	in   string
	want []keys.Key
}

var epoch = time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)

func TestDecoderStream(t *testing.T) {
	k := func(code rune, mod keys.Mod) keys.Key { return keys.Key{Code: code, Mod: mod} }
	esc := []keys.Key{k(keys.Escape, 0)}
	ms := time.Millisecond
	tests := []struct {
		name  string
		steps []step
	}{
		{name: "lone esc waits for the timeout", steps: []step{
			{at: 0, in: "\x1b"},
			{at: 49 * ms},
			{at: 50 * ms, want: esc},
			{at: 100 * ms},
		}},
		{name: "esc then a key within the timeout is alt", steps: []step{
			{at: 0, in: "\x1b"},
			{at: 10 * ms, in: "a", want: []keys.Key{k('a', keys.Alt)}},
			{at: 60 * ms},
		}},
		{name: "esc then legacy enter within the timeout is alt-enter", steps: []step{
			{at: 0, in: "\x1b"},
			{at: 10 * ms, in: "\r", want: []keys.Key{k(keys.Enter, keys.Alt)}},
		}},
		{name: "a read after the timeout ends the pending esc first", steps: []step{
			{at: 0, in: "\x1b"},
			{at: 70 * ms, in: "3", want: []keys.Key{k(keys.Escape, 0), k('3', 0)}},
		}},
		{name: "keys before a trailing esc come at once", steps: []step{
			{at: 0, in: "3\x1b", want: []keys.Key{k('3', 0)}},
			{at: 50 * ms, want: esc},
		}},
		{name: "kitty sequence split across reads", steps: []step{
			{at: 0, in: "\x1b[13;"},
			{at: 5 * ms, in: "5u", want: []keys.Key{k(keys.Enter, keys.Ctrl)}},
		}},
		{name: "kitty esc needs no timeout", steps: []step{
			{at: 0, in: "\x1b[27u", want: esc},
		}},
		{name: "unfinished sequence is unknown after the timeout", steps: []step{
			{at: 0, in: "\x1b[1"},
			{at: 50 * ms, want: []keys.Key{k(keys.Unknown, 0)}},
		}},
		{name: "two escs in one read are alt-esc", steps: []step{
			{at: 0, in: "\x1b\x1b", want: []keys.Key{k(keys.Escape, keys.Alt)}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := keys.NewDecoder(50 * ms)
			for i, s := range tt.steps {
				now := epoch.Add(s.at)
				var got []keys.Key
				if s.in != "" {
					got = d.Feed([]byte(s.in), now)
				} else {
					got = d.Expire(now)
				}
				if !reflect.DeepEqual(got, s.want) {
					t.Errorf("step %d: got %v, want %v", i, got, s.want)
				}
			}
		})
	}
}

func TestDecoderDeadline(t *testing.T) {
	d := keys.NewDecoder(50 * time.Millisecond)
	if _, ok := d.Deadline(); ok {
		t.Error("deadline before any input")
	}
	d.Feed([]byte("\x1b"), epoch)
	want := epoch.Add(50 * time.Millisecond)
	if dl, ok := d.Deadline(); !ok || !dl.Equal(want) {
		t.Errorf("deadline %v %v, want %v", dl, ok, want)
	}
	d.Expire(want)
	if _, ok := d.Deadline(); ok {
		t.Error("deadline after the esc expired")
	}
}
