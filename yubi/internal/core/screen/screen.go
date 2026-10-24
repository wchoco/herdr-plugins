// Package screen turns a herdr `pane read --source visible --format ansi`
// capture into rows of glyphs with display widths and original SGR styles.
//
// herdr sends one character per glyph and no spacer cells for wide glyphs,
// so column positions come from display widths computed here.
package screen

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// width measures glyphs like herdr's terminal does: East Asian ambiguous
// characters are narrow, whatever the locale of the process.
var width = func() *runewidth.Condition {
	c := runewidth.NewCondition()
	c.EastAsianWidth = false
	return c
}()

// RuneWidth returns the display width of r: 0 for zero-width runes such as
// combining marks, 2 for wide glyphs, 1 otherwise.
func RuneWidth(r rune) int { return width.RuneWidth(r) }

// Glyph is one character on the screen.
type Glyph struct {
	Text  string // one base rune plus any zero-width runes after it
	Width int    // display columns: 1 or 2
	SGR   string // original SGR parameters, e.g. "1;38;2;255;136;0"; "" is the default style
}

// Row is one screen row, left to right.
type Row []Glyph

// Screen is the Target Pane's visible screen as herdr returned it: one Row
// per screen row, trailing blank rows already dropped by herdr.
type Screen struct {
	Rows []Row
}

// Text returns the row's characters, including written trailing spaces.
func (r Row) Text() string {
	var b strings.Builder
	for _, g := range r {
		b.WriteString(g.Text)
	}
	return b.String()
}

// Width returns the row's width in display columns.
func (r Row) Width() int {
	w := 0
	for _, g := range r {
		w += g.Width
	}
	return w
}

// At returns the glyph that starts at display column col.
func (r Row) At(col int) (Glyph, bool) {
	c := 0
	for _, g := range r {
		if c == col {
			return g, true
		}
		c += g.Width
		if c > col {
			break
		}
	}
	return Glyph{}, false
}

// ParseANSI parses an ANSI-format visible read. Rows are separated by
// "\r\n" (a bare "\n" is accepted too). SGR sequences set the style of the
// glyphs after them; every other escape sequence and control byte is
// dropped.
func ParseANSI(s string) Screen {
	if s == "" {
		return Screen{}
	}
	var scr Screen
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		scr.Rows = append(scr.Rows, parseRow(line))
	}
	return scr
}

func parseRow(line string) Row {
	var row Row
	var sgr []string
	for i := 0; i < len(line); {
		if line[i] == 0x1b {
			n, params, isSGR := escape(line[i:])
			if isSGR {
				sgr = applySGR(sgr, params)
			}
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		i += size
		if r < 0x20 || r == 0x7f {
			continue
		}
		w := RuneWidth(r)
		if w == 0 {
			if len(row) > 0 {
				row[len(row)-1].Text += string(r)
			}
			continue
		}
		row = append(row, Glyph{Text: string(r), Width: w, SGR: strings.Join(sgr, ";")})
	}
	return row
}

// escape measures the escape sequence at the start of s. It reports the
// sequence's length and, for an SGR sequence (CSI ... m), its parameters.
func escape(s string) (n int, params string, isSGR bool) {
	if len(s) < 2 {
		return len(s), "", false
	}
	switch s[1] {
	case '[': // CSI: parameters and intermediates, then a final byte 0x40-0x7e
		for j := 2; j < len(s); j++ {
			if s[j] >= 0x40 && s[j] <= 0x7e {
				return j + 1, s[2:j], s[j] == 'm'
			}
		}
		return len(s), "", false
	case ']', 'P', '_', '^': // OSC, DCS, APC, PM: up to BEL or ST
		for j := 2; j < len(s); j++ {
			if s[j] == 0x07 {
				return j + 1, "", false
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
				return j + 2, "", false
			}
		}
		return len(s), "", false
	default:
		return 2, "", false
	}
}

// applySGR folds one SGR parameter list into the active parameters. A reset
// (0 or an empty list) clears them; extended colours (38/48/58 with 5 or 2)
// are kept whole, so their zero components are not taken as resets.
func applySGR(active []string, params string) []string {
	if params == "" {
		return nil
	}
	ps := strings.Split(params, ";")
	for i := 0; i < len(ps); i++ {
		p := ps[i]
		if p == "0" || p == "" {
			active = nil
			continue
		}
		if p == "38" || p == "48" || p == "58" {
			n := 0
			if i+1 < len(ps) {
				switch ps[i+1] {
				case "5":
					n = 2
				case "2":
					n = 4
				}
			}
			end := min(i+1+n, len(ps))
			active = append(active, strings.Join(ps[i:end], ";"))
			i = end - 1
			continue
		}
		if _, err := strconv.Atoi(p); err == nil {
			active = append(active, p)
		}
	}
	return active
}
