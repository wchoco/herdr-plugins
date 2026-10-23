package keys_test

import (
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/keys"
)

// Keys that legacy terminals send as the same bytes share one Canon form,
// whichever way they were decoded.
func TestCanon(t *testing.T) {
	k := func(code rune, mod keys.Mod) keys.Key { return keys.Key{Code: code, Mod: mod} }
	tests := []struct {
		name  string
		in    string   // bytes from the terminal
		named keys.Key // the same key as the config spells it
		want  keys.Key
	}{
		{name: "enter is ctrl-m", in: "\r", named: k('m', keys.Ctrl), want: k(keys.Enter, 0)},
		{name: "tab is ctrl-i", in: "\t", named: k('i', keys.Ctrl), want: k(keys.Tab, 0)},
		{name: "ctrl-space is NUL", in: "\x00", named: k(' ', keys.Ctrl), want: k(' ', keys.Ctrl)},
		{name: "shift-a is A", in: "A", named: k('a', keys.Shift), want: k('A', 0)},
		{name: "ctrl-j", in: "\n", named: k('j', keys.Ctrl), want: k('j', keys.Ctrl)},
		{name: "ctrl-enter stays apart from enter", in: "\x1b[13;5u", named: k(keys.Enter, keys.Ctrl), want: k(keys.Enter, keys.Ctrl)},
		{name: "alt is kept", in: "\x1b\r", named: k('m', keys.Ctrl|keys.Alt), want: k(keys.Enter, keys.Alt)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ks := keys.Decode([]byte(tt.in))
			if len(ks) != 1 {
				t.Fatalf("Decode(%q) = %v", tt.in, ks)
			}
			if got := keys.Canon(ks[0]); got != tt.want {
				t.Errorf("Canon(Decode(%q)) = %v, want %v", tt.in, got, tt.want)
			}
			if got := keys.Canon(tt.named); got != tt.want {
				t.Errorf("Canon(%v) = %v, want %v", tt.named, got, tt.want)
			}
		})
	}
}
