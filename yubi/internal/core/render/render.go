// Package render draws yubi's overlay frames into a cell grid and encodes
// the grid as terminal output.
//
// A Grid is the overlay's screen, one Cell per terminal cell. A wide glyph
// occupies its first cell, and the cell after it is a continuation. Drawing
// over either half of a wide glyph replaces the other half with a space, so
// columns never shift.
package render

import (
	"strconv"
	"strings"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
)

// Role says what a cell shows, which decides its final style.
type Role byte

const (
	Text       Role = iota // Target Pane text in its original style
	LineNumber             // a Line Number label: bold black on cyan
	Status                 // the status row: reverse video
)

// sgr is the fixed style of each yubi role; Text keeps the cell's original SGR.
var sgr = map[Role]string{
	LineNumber: "1;30;46",
	Status:     "7",
}

// letter is the role's letter in a Dump style map. Text cells are '.' in
// the default style and 'o' in any other original style.
var letter = map[Role]byte{
	LineNumber: 'n',
	Status:     's',
}

// Cell is one terminal cell.
type Cell struct {
	Text  string // "" for the continuation cell of a wide glyph
	Width int    // 2 for a wide glyph's first cell, 1 otherwise, 0 for a continuation
	SGR   string // original SGR parameters of Text cells
	Role  Role
}

func blank() Cell { return Cell{Text: " ", Width: 1} }

// Grid is a frame of W x H cells.
type Grid struct {
	W, H  int
	cells [][]Cell
}

// NewGrid returns a blank grid.
func NewGrid(w, h int) *Grid {
	g := &Grid{W: w, H: h, cells: make([][]Cell, h)}
	for y := range g.cells {
		g.cells[y] = make([]Cell, w)
		for x := range g.cells[y] {
			g.cells[y][x] = blank()
		}
	}
	return g
}

// DrawRow draws a screen row at column x of row y in its original style,
// clipped at the right edge.
func (g *Grid) DrawRow(y, x int, r screen.Row) {
	for _, gl := range r {
		if x >= g.W {
			return
		}
		g.put(y, x, Cell{Text: gl.Text, Width: gl.Width, SGR: gl.SGR, Role: Text})
		x += gl.Width
	}
}

// Label draws text over the cells from column x of row y in role's style,
// clipped at the right edge.
func (g *Grid) Label(y, x int, text string, role Role) {
	for _, r := range text {
		w := screen.RuneWidth(r)
		if w == 0 {
			continue
		}
		if x >= g.W {
			return
		}
		g.put(y, x, Cell{Text: string(r), Width: w, Role: role})
		x += w
	}
}

// Fill paints n cells from column x of row y as blanks in role's style.
func (g *Grid) Fill(y, x, n int, role Role) {
	for i := x; i < x+n && i < g.W; i++ {
		g.put(y, i, Cell{Text: " ", Width: 1, Role: role})
	}
}

// Cell returns the cell at column x of row y.
func (g *Grid) Cell(y, x int) Cell { return g.cells[y][x] }

// put writes c at (y, x). Any wide glyph it partly covers loses its other
// half to a space in that glyph's style; a wide c that does not fit before
// the right edge becomes a space.
func (g *Grid) put(y, x int, c Cell) {
	if y < 0 || y >= g.H || x < 0 || x >= g.W {
		return
	}
	if c.Width == 2 && x+1 >= g.W {
		c.Text, c.Width = " ", 1
	}
	g.clear(y, x)
	if c.Width == 2 {
		g.clear(y, x+1)
		g.cells[y][x+1] = Cell{SGR: c.SGR, Role: c.Role}
	}
	g.cells[y][x] = c
}

// clear breaks up the wide glyph that covers (y, x), if any, leaving a
// space in its style on the half that is not (y, x).
func (g *Grid) clear(y, x int) {
	row := g.cells[y]
	switch c := row[x]; {
	case c.Width == 2 && x+1 < g.W:
		row[x+1] = Cell{Text: " ", Width: 1, SGR: c.SGR, Role: c.Role}
	case c.Width == 0 && x > 0:
		left := row[x-1]
		row[x-1] = Cell{Text: " ", Width: 1, SGR: left.SGR, Role: left.Role}
	}
}

// style returns the SGR parameters a cell is drawn with.
func (c Cell) style() string {
	if c.Role == Text {
		return c.SGR
	}
	return sgr[c.Role]
}

// RowText returns the text of row y, one character per cell.
func (g *Grid) RowText(y int) string {
	var b strings.Builder
	for _, c := range g.cells[y] {
		b.WriteString(c.Text)
	}
	return b.String()
}

// Dump returns a human-readable dump of the grid for golden tests: the
// cells' text, then a style map with one letter per cell. Each line ends
// with '|' so trailing spaces stay visible.
func (g *Grid) Dump() string {
	var b strings.Builder
	b.WriteString("text:\n")
	for y := range g.cells {
		b.WriteString(g.RowText(y) + "|\n")
	}
	b.WriteString("style:\n")
	for _, row := range g.cells {
		for _, c := range row {
			switch {
			case c.Role != Text:
				b.WriteByte(letter[c.Role])
			case c.SGR != "":
				b.WriteByte('o')
			default:
				b.WriteByte('.')
			}
		}
		b.WriteString("|\n")
	}
	return b.String()
}

// Encode returns the terminal output that draws the whole grid: each row
// is positioned absolutely, and SGR is emitted only where the style changes.
func (g *Grid) Encode() []byte {
	var b strings.Builder
	for y, row := range g.cells {
		b.WriteString("\x1b[" + strconv.Itoa(y+1) + ";1H")
		cur := "-" // no style emitted yet on this row
		for _, c := range row {
			if c.Width == 0 {
				continue
			}
			if s := c.style(); s != cur {
				if s == "" {
					b.WriteString("\x1b[0m")
				} else {
					b.WriteString("\x1b[0;" + s + "m")
				}
				cur = s
			}
			b.WriteString(c.Text)
		}
	}
	b.WriteString("\x1b[0m")
	return []byte(b.String())
}
