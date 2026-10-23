package render_test

import (
	"strings"
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/render"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
)

func row(ansi string) screen.Row { return screen.ParseANSI(ansi).Rows[0] }

func dump(lines ...string) string { return strings.Join(lines, "\n") + "\n" }

func TestDrawRowPlacesWideGlyphsAndClipsRight(t *testing.T) {
	tests := []struct {
		name string
		w    int
		x    int
		in   string
		want string
	}{
		{name: "ascii", w: 6, in: "ab", want: dump("text:", "ab    |", "style:", "......|")},
		{name: "wide glyphs take two cells", w: 6, in: "a漢b", want: dump("text:", "a漢b  |", "style:", "......|")},
		{name: "clipped at the right edge", w: 3, in: "abcdef", want: dump("text:", "abc|", "style:", "...|")},
		{name: "wide glyph cut by the edge becomes a space", w: 4, in: "abc漢", want: dump("text:", "abc |", "style:", "....|")},
		{name: "offset", w: 5, x: 2, in: "\x1b[31mab", want: dump("text:", "  ab |", "style:", "..oo.|")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := render.NewGrid(tt.w, 1)
			g.DrawRow(0, tt.x, row(tt.in))
			if got := g.Dump(); got != tt.want {
				t.Errorf("got\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestLabelOverWideGlyphHalves(t *testing.T) {
	tests := []struct {
		name string
		x    int
		text string
		want string
	}{
		{name: "label ends on the left half", x: 0, text: "12", want: dump("text:", "12 b  |", "style:", "nno...|")},
		{name: "label starts on the right half", x: 2, text: "9", want: dump("text:", "a 9b  |", "style:", ".on...|")},
		{name: "label covers the whole glyph", x: 1, text: "77", want: dump("text:", "a77b  |", "style:", ".nn...|")},
		{name: "label clipped at the edge", x: 5, text: "123", want: dump("text:", "a漢b 1|", "style:", ".oo..n|")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := render.NewGrid(6, 1)
			g.DrawRow(0, 0, row("a\x1b[1m漢\x1b[0mb"))
			g.Label(0, tt.x, tt.text, render.LineNumber)
			if got := g.Dump(); got != tt.want {
				t.Errorf("got\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestRestyleCombinesWithOriginalStyle(t *testing.T) {
	g := render.NewGrid(6, 1)
	g.DrawRow(0, 0, row("a\x1b[31mbc\x1b[0md"))
	g.Restyle(0, 0, 6, render.Dim)
	g.Restyle(0, 1, 2, render.Match)
	g.Label(0, 1, "h", render.Hint)
	g.Label(0, 4, "p", render.HintTyped)
	want := dump("text:", "ahcdp |", "style:", "dhudpd|")
	if got := g.Dump(); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	wantEnc := "\x1b[1;1H\x1b[0;2ma\x1b[0;1;30;43mh\x1b[0;31;4mc\x1b[0;2md\x1b[0;2;30;43mp\x1b[0;2m \x1b[0m"
	if got := string(g.Encode()); got != wantEnc {
		t.Errorf("got  %q\nwant %q", got, wantEnc)
	}
}

func TestSelectReversesTheWholeRowOverItsOriginalStyle(t *testing.T) {
	g := render.NewGrid(6, 1)
	g.DrawRow(0, 0, row("a\x1b[31m漢\x1b[0mb"))
	g.Label(0, 0, "2", render.LineNumber)
	g.Select(0)
	want := dump("text:", "2漢b  |", "style:", "nrrrrr|")
	if got := g.Dump(); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	// Reverse video is added to the original SGR; the label keeps its style.
	wantEnc := "\x1b[1;1H\x1b[0;1;30;46m2\x1b[0;31;7m漢\x1b[0;7mb  \x1b[0m"
	if got := string(g.Encode()); got != wantEnc {
		t.Errorf("got  %q\nwant %q", got, wantEnc)
	}
}

func TestErrorRoleInStatusRow(t *testing.T) {
	g := render.NewGrid(6, 1)
	g.Fill(0, 0, 6, render.Status)
	g.Label(0, 0, "L", render.Status)
	g.Label(0, 2, "no", render.Error)
	want := dump("text:", "L no  |", "style:", "sseess|")
	if got := g.Dump(); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	wantEnc := "\x1b[1;1H\x1b[0;7mL \x1b[0;7;1;31mno\x1b[0;7m  \x1b[0m"
	if got := string(g.Encode()); got != wantEnc {
		t.Errorf("got  %q\nwant %q", got, wantEnc)
	}
}

func TestEncode(t *testing.T) {
	g := render.NewGrid(5, 2)
	g.DrawRow(0, 0, row("a\x1b[38;2;1;2;3m漢\x1b[0mb"))
	g.Label(1, 0, "3", render.LineNumber)
	g.Fill(1, 2, 3, render.Status)
	g.Label(1, 2, "LI", render.Status)
	want := "" +
		"\x1b[1;1H\x1b[0ma\x1b[0;38;2;1;2;3m漢\x1b[0mb " +
		"\x1b[2;1H\x1b[0;1;30;46m3\x1b[0m \x1b[0;7mLI " +
		"\x1b[0m"
	if got := string(g.Encode()); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}
