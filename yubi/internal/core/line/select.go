package line

import (
	"strconv"
	"strings"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
)

// OutOfRangeError reports a Line Number above the top displayed row.
type OutOfRangeError struct{ Number int }

func (e OutOfRangeError) Error() string { return "no line " + strconv.Itoa(e.Number) }

// Select returns the rows ranges select, top to bottom and deduplicated.
// It is all-or-nothing: a Line Number above the top row is an
// OutOfRangeError and selects nothing; there is no clamping.
func (n Numbering) Select(ranges []Range) ([]int, error) {
	for _, r := range ranges {
		if r.To > n.Max() {
			return nil, OutOfRangeError{Number: r.To}
		}
	}
	return n.Rows(ranges), nil
}

// Rows returns the rows ranges select, top to bottom and deduplicated,
// leaving out Line Numbers that are not on screen. It backs the live
// preview, where the input may still be out of range.
func (n Numbering) Rows(ranges []Range) []int {
	selected := make([]bool, n.Max()+1) // by Line Number
	for _, r := range ranges {
		for num := max(r.From, 1); num <= min(r.To, n.Max()); num++ {
			selected[num] = true
		}
	}
	var rows []int
	for num := n.Max(); num >= 1; num-- {
		if selected[num] {
			row, _ := n.Row(num)
			rows = append(rows, row)
		}
	}
	return rows
}

// Copy returns the copy text of rows: each row's Text, joined by "\n", with
// no trailing newline. A blank row becomes an empty line.
func Copy(scr screen.Screen, rows []int) string {
	lines := make([]string, len(rows))
	for i, row := range rows {
		lines[i] = Text(scr, row)
	}
	return strings.Join(lines, "\n")
}

// JoinedCopy returns the Joined Copy text of rows: the same rows as Copy,
// joined by sep. Intermediate rows keep their trailing whitespace as herdr
// wrote it, so a soft wrap that fell on a space keeps that space; only the
// last row is trimmed.
func JoinedCopy(scr screen.Screen, rows []int, sep string) string {
	lines := make([]string, len(rows))
	for i, row := range rows {
		if i == len(rows)-1 {
			lines[i] = Text(scr, row)
		} else {
			lines[i] = scr.Rows[row].Text()
		}
	}
	return strings.Join(lines, sep)
}
