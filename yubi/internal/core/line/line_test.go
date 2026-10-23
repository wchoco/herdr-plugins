package line_test

import (
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/line"
	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
	"github.com/wchoco/herdr-plugins/yubi/internal/fixture"
)

func TestNumbersCountUpwardFromLastNonBlankDisplayedRow(t *testing.T) {
	shell := screen.ParseANSI(fixture.Load(t, "shell").ANSI)
	cjk := screen.ParseANSI(fixture.Load(t, "cjk").ANSI)
	tests := []struct {
		name      string
		scr       screen.Screen
		displayed int // rows yubi displays
		max       int // number on the top row
		number    int
		text      string // copy text of that Line Number
	}{
		{name: "line 1 is the last row", scr: shell, displayed: 39, max: 19, number: 1, text: "user@host:~/proj$"},
		{name: "blank rows are numbered", scr: shell, displayed: 39, max: 19, number: 2, text: ""},
		{name: "trailing whitespace trimmed", scr: shell, displayed: 39, max: 19, number: 4, text: "trailing spaces"},
		{name: "top row", scr: shell, displayed: 39, max: 19, number: 19, text: "user@host:~/proj$ git status"},
		{name: "clipped rows: line 1 skips trailing blank rows", scr: shell, displayed: 10, max: 9, number: 1, text: "        yubi/testdata/"},
		{name: "wide glyphs", scr: cjk, displayed: 39, max: 7, number: 6, text: "日本語のテキスト: wide glyphs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := line.NewNumbering(tt.scr, tt.displayed)
			if n.Max() != tt.max {
				t.Errorf("Max: got %d, want %d", n.Max(), tt.max)
			}
			row, ok := n.Row(tt.number)
			if !ok {
				t.Fatalf("Line Number %d not found", tt.number)
			}
			if got := n.Of(row); got != tt.number {
				t.Errorf("Of(%d): got %d, want %d", row, got, tt.number)
			}
			if got := line.Text(tt.scr, row); got != tt.text {
				t.Errorf("Text: got %q, want %q", got, tt.text)
			}
		})
	}
}

func TestNumbersOutOfRange(t *testing.T) {
	scr := screen.ParseANSI("a\r\n\r\nb")
	n := line.NewNumbering(scr, 10)
	for _, number := range []int{0, -1, 4} {
		if _, ok := n.Row(number); ok {
			t.Errorf("Row(%d) found", number)
		}
	}
	if got := n.Of(5); got != 0 {
		t.Errorf("row below the screen has number %d", got)
	}
}

func TestNumbersOnBlankScreen(t *testing.T) {
	for _, in := range []string{"", "   \r\n  "} {
		n := line.NewNumbering(screen.ParseANSI(in), 10)
		if n.Max() != 0 {
			t.Errorf("%q: Max %d, want 0", in, n.Max())
		}
	}
}
