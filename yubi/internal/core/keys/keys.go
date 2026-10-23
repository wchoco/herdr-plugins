// Package keys decodes the bytes yubi's pane reads from its terminal into
// key events. It accepts both legacy encodings and the kitty keyboard
// protocol with flag 1 (disambiguate), which yubi's pane requests.
package keys

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// Mod is a set of key modifiers.
type Mod uint8

const (
	Shift Mod = 1 << iota
	Alt
	Ctrl
)

// Codes of non-printing keys, as in the kitty keyboard protocol.
const (
	Unknown   rune = -1
	Enter     rune = 13
	Escape    rune = 27
	Backspace rune = 127
)

// Key is one key press. Code is the key's Unicode code point (lowercase
// for Ctrl chords) or one of the codes above.
type Key struct {
	Code rune
	Mod  Mod
}

// Decode splits one read into key presses.
func Decode(b []byte) []Key {
	var out []Key
	for len(b) > 0 {
		k, n := decodeOne(b)
		out = append(out, k)
		b = b[n:]
	}
	return out
}

func decodeOne(b []byte) (Key, int) {
	switch c := b[0]; {
	case c == 0x1b:
		if len(b) == 1 {
			return Key{Code: Escape}, 1
		}
		if b[1] == '[' {
			return csi(b)
		}
		k, n := decodeOne(b[1:])
		k.Mod |= Alt
		return k, n + 1
	case c == '\r':
		return Key{Code: Enter}, 1
	case c == 0x7f:
		return Key{Code: Backspace}, 1
	case c < 0x20:
		return Key{Code: rune(c) + 'a' - 1, Mod: Ctrl}, 1
	}
	r, n := utf8.DecodeRune(b)
	return Key{Code: r}, n
}

// csi decodes a CSI sequence. Only kitty `CSI code[;mods] u` is a known key.
func csi(b []byte) (Key, int) {
	end := 2
	for end < len(b) && (b[end] < 0x40 || b[end] > 0x7e) {
		end++
	}
	if end == len(b) {
		return Key{Code: Unknown}, len(b)
	}
	n := end + 1
	if b[end] != 'u' {
		return Key{Code: Unknown}, n
	}
	fields := strings.Split(string(b[2:end]), ";")
	code, err := strconv.Atoi(strings.Split(fields[0], ":")[0])
	if err != nil {
		return Key{Code: Unknown}, n
	}
	k := Key{Code: rune(code)}
	if len(fields) > 1 {
		m, err := strconv.Atoi(strings.Split(fields[1], ":")[0])
		if err != nil || m < 1 {
			return Key{Code: Unknown}, n
		}
		k.Mod = Mod(m - 1)
	}
	return k, n
}
