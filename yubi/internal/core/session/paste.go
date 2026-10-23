package session

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// PasteText is text as yubi pastes it: control characters are removed,
// except newline and tab, so screen text cannot inject terminal sequences
// into the Target Pane. Bytes that are not valid UTF-8 are removed too.
func PasteText(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		i += n
		if r == utf8.RuneError && n == 1 {
			continue
		}
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
