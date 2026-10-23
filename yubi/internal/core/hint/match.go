package hint

import (
	"regexp"
	"strings"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
)

// Pattern is a named regex that finds Matches. When the regex has a group
// named `match`, that group is what the Match copies; otherwise the whole
// match is.
type Pattern struct {
	Name string
	Re   *regexp.Regexp
}

// builtins are the Built-in Patterns in priority order, highest first.
var builtins = []Pattern{
	{"url", regexp.MustCompile(`(?P<match>(https?://|git@|git://|ssh://|ftp://|file:///)[^ ]+)`)},
	{"path", regexp.MustCompile(`(?P<match>([.\w\-@$~\[\]]+)?(/[.\w\-@$\[\]]+)+)`)},
}

// Builtins returns the Built-in Patterns in priority order, highest first.
// Custom Patterns go before them in the list passed to Find.
func Builtins() []Pattern { return append([]Pattern(nil), builtins...) }

// Match is one Pattern match on the screen. Col and Width give the cells of
// the copied text, in display columns.
type Match struct {
	Row, Col, Width int
	Text            string // the text the Match copies
	Pattern         string // name of the Pattern that found it
}

// Find returns the Matches on rows in reading order. Each row is scanned
// left to right: the leftmost match of any Pattern wins, a tie goes to the
// Pattern earlier in pats, and the scan goes on after the winning match, so
// Matches never overlap.
func Find(rows []screen.Row, pats []Pattern) []Match {
	var out []Match
	for y, row := range rows {
		out = append(out, findRow(y, row, pats)...)
	}
	return out
}

func findRow(y int, row screen.Row, pats []Pattern) []Match {
	// The row's text, and for each glyph its byte offset and display column.
	var b strings.Builder
	var offs, cols []int
	col := 0
	for _, g := range row {
		offs = append(offs, b.Len())
		cols = append(cols, col)
		b.WriteString(g.Text)
		col += g.Width
	}
	text := b.String()
	// glyph returns the index of the glyph holding byte i.
	glyph := func(i int) int {
		j := len(offs) - 1
		for j > 0 && offs[j] > i {
			j--
		}
		return j
	}

	var out []Match
	for pos := 0; pos < len(text); {
		var best []int // submatch indices relative to text
		var bestPat Pattern
		for _, p := range pats {
			loc := firstNonEmpty(p.Re, text[pos:])
			if loc == nil {
				continue
			}
			if best == nil || loc[0]+pos < best[0] {
				for i := range loc {
					if loc[i] >= 0 {
						loc[i] += pos
					}
				}
				best, bestPat = loc, p
			}
		}
		if best == nil {
			break
		}
		start, end := best[0], best[1]
		if g := bestPat.Re.SubexpIndex("match"); g > 0 && best[2*g] >= 0 && best[2*g+1] > best[2*g] {
			start, end = best[2*g], best[2*g+1]
		}
		first, last := glyph(start), glyph(end-1)
		out = append(out, Match{
			Row:     y,
			Col:     cols[first],
			Width:   cols[last] + row[last].Width - cols[first],
			Text:    text[start:end],
			Pattern: bestPat.Name,
		})
		pos = best[1]
	}
	return out
}

// firstNonEmpty returns the submatch indices of re's first non-empty match
// in s, or nil.
func firstNonEmpty(re *regexp.Regexp, s string) []int {
	if loc := re.FindStringSubmatchIndex(s); loc == nil || loc[1] > loc[0] {
		return loc
	}
	for _, loc := range re.FindAllStringSubmatchIndex(s, -1) {
		if loc[1] > loc[0] {
			return loc
		}
	}
	return nil
}
