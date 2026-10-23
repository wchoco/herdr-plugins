package keys_test

import (
	"reflect"
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/keys"
)

func TestDecode(t *testing.T) {
	k := func(code rune, mod keys.Mod) keys.Key { return keys.Key{Code: code, Mod: mod} }
	tests := []struct {
		name string
		in   string
		want []keys.Key
	}{
		{name: "digits", in: "309", want: []keys.Key{k('3', 0), k('0', 0), k('9', 0)}},
		{name: "legacy enter", in: "\r", want: []keys.Key{k(keys.Enter, 0)}},
		{name: "digit then enter in one read", in: "3\r", want: []keys.Key{k('3', 0), k(keys.Enter, 0)}},
		{name: "legacy ctrl-j", in: "\n", want: []keys.Key{k('j', keys.Ctrl)}},
		{name: "legacy ctrl-c", in: "\x03", want: []keys.Key{k('c', keys.Ctrl)}},
		{name: "legacy alt-enter", in: "\x1b\r", want: []keys.Key{k(keys.Enter, keys.Alt)}},
		{name: "legacy backspace", in: "\x7f", want: []keys.Key{k(keys.Backspace, 0)}},
		{name: "kitty enter", in: "\x1b[13u", want: []keys.Key{k(keys.Enter, 0)}},
		{name: "kitty ctrl-enter", in: "\x1b[13;5u", want: []keys.Key{k(keys.Enter, keys.Ctrl)}},
		{name: "kitty alt-enter", in: "\x1b[13;3u", want: []keys.Key{k(keys.Enter, keys.Alt)}},
		{name: "kitty ctrl-j", in: "\x1b[106;5u", want: []keys.Key{k('j', keys.Ctrl)}},
		{name: "kitty esc", in: "\x1b[27u", want: []keys.Key{k(keys.Escape, 0)}},
		{name: "kitty ctrl-c", in: "\x1b[99;5u", want: []keys.Key{k('c', keys.Ctrl)}},
		{name: "kitty sequence then digit", in: "\x1b[27u1", want: []keys.Key{k(keys.Escape, 0), k('1', 0)}},
		{name: "unknown sequence", in: "\x1b[A4", want: []keys.Key{k(keys.Unknown, 0), k('4', 0)}},
		{name: "utf-8 text", in: "漢", want: []keys.Key{k('漢', 0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keys.Decode([]byte(tt.in)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Decode(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
