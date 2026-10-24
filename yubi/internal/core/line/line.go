// Package line implements Line Mode's pure parts: Line Numbers and the
// text a selection of lines copies.
package line

import (
	"strings"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
)

// Numbering maps screen rows to Line Numbers. Line Number 1 is the last
// non-blank row yubi displays; numbers count upward from there, blank rows
// included. Rows below Line 1 have no number.
type Numbering struct {
	last int // row index of Line Number 1, -1 when nothing displayed is non-blank
}

// NewNumbering numbers the first displayed rows of scr.
func NewNumbering(scr screen.Screen, displayed int) Numbering {
	n := Numbering{last: -1}
	for i := min(displayed, len(scr.Rows)) - 1; i >= 0; i-- {
		if strings.TrimSpace(scr.Rows[i].Text()) != "" {
			n.last = i
			break
		}
	}
	return n
}

// Max returns the number of the top row, which is also the widest number;
// 0 when there are no Line Numbers.
func (n Numbering) Max() int { return n.last + 1 }

// Of returns the Line Number of row, or 0 when the row has none.
func (n Numbering) Of(row int) int {
	if row < 0 || row > n.last {
		return 0
	}
	return n.last - row + 1
}

// Row returns the row index that carries Line Number number.
func (n Numbering) Row(number int) (int, bool) {
	if number < 1 || number > n.Max() {
		return 0, false
	}
	return n.last - number + 1, true
}

// Text returns the copy text of one row: its characters with trailing
// whitespace trimmed and no newline.
func Text(scr screen.Screen, row int) string {
	return strings.TrimRight(scr.Rows[row].Text(), " \t")
}
