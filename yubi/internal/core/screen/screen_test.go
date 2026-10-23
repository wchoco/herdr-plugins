package screen_test

import (
	"strings"
	"testing"

	"github.com/wchoco/herdr-plugins/yubi/internal/core/screen"
	"github.com/wchoco/herdr-plugins/yubi/internal/fixture"
)

// The ANSI read, with SGR dropped and trailing whitespace trimmed, carries
// the same rows as herdr's plain read.
func TestParseANSIMatchesPlainRead(t *testing.T) {
	for _, c := range fixture.All(t) {
		t.Run(c.Name, func(t *testing.T) {
			s := screen.ParseANSI(c.ANSI)
			want := strings.Split(strings.TrimSuffix(c.Text, "\n"), "\n")
			if len(s.Rows) != len(want) {
				t.Fatalf("got %d rows, want %d", len(s.Rows), len(want))
			}
			for i, row := range s.Rows {
				if got := strings.TrimRight(row.Text(), " \t"); got != want[i] {
					t.Errorf("row %d: got %q, want %q", i, got, want[i])
				}
			}
		})
	}
}

func TestParseANSIKeepsWrittenTrailingSpaces(t *testing.T) {
	s := screen.ParseANSI(fixture.Load(t, "shell").ANSI)
	if got := s.Rows[15].Text(); got != "trailing spaces   " {
		t.Fatalf("got %q", got)
	}
}

// Wide glyphs take two columns; herdr sends no spacer cells for them.
func TestParseANSIDisplayWidths(t *testing.T) {
	s := screen.ParseANSI(fixture.Load(t, "cjk").ANSI)
	tests := []struct {
		row      int
		col      int // column of the glyph
		glyph    string
		width    int
		rowWidth int
	}{
		{row: 1, col: 0, glyph: "日", width: 2, rowWidth: 29},
		{row: 1, col: 16, glyph: ":", width: 1, rowWidth: 29},
		{row: 2, col: 12, glyph: "漢", width: 2, rowWidth: 62},
		{row: 2, col: 58, glyph: "路", width: 2, rowWidth: 62},
		{row: 4, col: 6, glyph: "🎉", width: 2, rowWidth: 14},
		{row: 4, col: 9, glyph: "p", width: 1, rowWidth: 14},
	}
	for _, tt := range tests {
		row := s.Rows[tt.row]
		if got := row.Width(); got != tt.rowWidth {
			t.Errorf("row %d width: got %d, want %d", tt.row, got, tt.rowWidth)
		}
		g, ok := row.At(tt.col)
		if !ok || g.Text != tt.glyph || g.Width != tt.width {
			t.Errorf("row %d col %d: got %+v (found %v), want %q width %d", tt.row, tt.col, g, ok, tt.glyph, tt.width)
		}
	}
}

// Original styles are kept per glyph as SGR parameters, including
// truecolor and 256-colour.
func TestParseANSIKeepsSGR(t *testing.T) {
	s := screen.ParseANSI(fixture.Load(t, "cjk").ANSI)
	tests := []struct {
		row, col int
		sgr      string
	}{
		{row: 0, col: 0, sgr: "38;5;2"},
		{row: 0, col: 9, sgr: ""},
		{row: 3, col: 0, sgr: "38;2;255;136;0"},
		{row: 3, col: 10, sgr: "38;2;255;136;0"},
		{row: 3, col: 13, sgr: ""},
		{row: 3, col: 17, sgr: "38;5;45"},
		{row: 5, col: 0, sgr: "7"},
	}
	for _, tt := range tests {
		g, _ := s.Rows[tt.row].At(tt.col)
		if g.SGR != tt.sgr {
			t.Errorf("row %d col %d %q: got SGR %q, want %q", tt.row, tt.col, g.Text, g.SGR, tt.sgr)
		}
	}
}

func TestParseANSIHandWritten(t *testing.T) {
	tests := []struct {
		name string
		in   string
		rows []string
		sgr  []string // SGR of each glyph of the first row
	}{
		{name: "empty", in: "", rows: nil},
		{name: "reset inside sequence", in: "\x1b[1;31ma\x1b[0;32mb\x1b[mc", rows: []string{"abc"}, sgr: []string{"1;31", "32", ""}},
		{name: "colour with zero component is not a reset", in: "\x1b[1;38;2;0;0;0ma", rows: []string{"a"}, sgr: []string{"1;38;2;0;0;0"}},
		{name: "non-SGR escapes are dropped", in: "\x1b]8;;http://x\x1b\\a\x1b[2Kb", rows: []string{"ab"}, sgr: []string{"", ""}},
		{name: "blank rows kept", in: "a\r\n\r\nb", rows: []string{"a", "", "b"}},
		{name: "combining mark joins its base", in: "éx", rows: []string{"éx"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := screen.ParseANSI(tt.in)
			var got []string
			for _, r := range s.Rows {
				got = append(got, r.Text())
			}
			if strings.Join(got, "|") != strings.Join(tt.rows, "|") || len(got) != len(tt.rows) {
				t.Fatalf("rows: got %q, want %q", got, tt.rows)
			}
			for i, want := range tt.sgr {
				if g := s.Rows[0][i]; g.SGR != want {
					t.Errorf("glyph %d %q: got SGR %q, want %q", i, g.Text, g.SGR, want)
				}
			}
		})
	}
}
